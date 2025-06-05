package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	rcs "github.com/wecredit/communication-sdk/sdk/channels/rcs"
	sms "github.com/wecredit/communication-sdk/sdk/channels/sms"
	"github.com/wecredit/communication-sdk/sdk/channels/whatsapp"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/sdk/models/awsModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

type MessageWrapper struct {
	Message *sqs.Message
}

// ConsumerService starts the SQS consumer service using workers
func ConsumerService(workerCount int, queueURL string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgChan := make(chan MessageWrapper, workerCount)
	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(ctx, queue.SQSClient, queueURL, msgChan, &wg)
	}

	// Shutdown handler
	go handleShutdown(cancel)

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping message poller")
			close(msgChan)
			wg.Wait()
			return
		default:
			// Long poll SQS for messages
			result, err := queue.SQSClient.ReceiveMessage(&sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queueURL),
				MaxNumberOfMessages: aws.Int64(int64(workerCount)),
				WaitTimeSeconds:     aws.Int64(10),
				VisibilityTimeout:   aws.Int64(30),
			})
			if err != nil {
				log.Printf("Error receiving messages: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			for _, msg := range result.Messages {
				msgChan <- MessageWrapper{Message: msg}
			}
		}
	}
}

func worker(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msgChan <-chan MessageWrapper, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			log.Println("Worker context cancelled")
			return
		case msgWrapper, ok := <-msgChan:
			if !ok {
				return
			}
			processMessage(ctx, sqsClient, queueURL, msgWrapper.Message)
		}
	}
}

// handleShutdown listens for termination signals and shuts down gracefully
func handleShutdown(cancelFunc context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received shutdown signal: %v", sig)
	cancelFunc()
}

func processMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) {
	select {
	case <-ctx.Done():
		log.Println("Context cancelled. Skipping message processing.")
		return
	default:
		// Proceed with message processing
	}

	var snsWrapper awsModels.SnsMessageWrapper
	if err := json.Unmarshal([]byte(*msg.Body), &snsWrapper); err != nil {
		log.Printf("Failed to unmarshal SNS wrapper: %v", err)
		return
	}

	var data sdkModels.CommApiRequestBody
	if err := json.Unmarshal([]byte(snsWrapper.Message), &data); err != nil {
		log.Printf("Failed to unmarshal inner message: %v", err)
		return
	}

	data.Client = strings.ToLower(data.Client)
	data.Channel = strings.ToUpper(data.Channel)
	data.ProcessName = strings.ToUpper(data.ProcessName)
	data.Vendor = strings.ToUpper(data.Vendor)
	// data.Client = strings.ToUpper(data.Client)

	dbMappedData, err := services.MapIntoDbModel(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	utils.Debug(fmt.Sprintf("Data: %v", data))

	switch data.Channel {
	case variables.WhatsApp:
		if err := database.InsertData(config.Configs.SdkWhatsappInputTable, database.DBtech, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		}

		if data.Client == variables.CreditSea {
			data.Vendor = variables.SINCH
		} else {
			data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
		}
		
		response, err := whatsapp.SendWpByProcess(data)
		if err == nil {
			utils.Debug(fmt.Sprintf("%v", response))
		} else {
			utils.Error(fmt.Errorf("error in sending whatsapp: %v", err))
			return
		}
	case variables.RCS:
		if err := database.InsertData(config.Configs.SdkRcsInputTable, database.DBtech, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		}
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
		response, err := rcs.SendRcsByProcess(data)
		if err == nil {
			utils.Debug(fmt.Sprintf("%v", response))
		} else {
			utils.Error(fmt.Errorf("error in sending RCS: %v", err))
			return
		}

	case variables.SMS:
		// return false
		if err := database.InsertData(config.Configs.SdkSmsInputTable, database.DBtech, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		}
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
		response, err := sms.SendSmsByProcess(data)
		if err == nil {
			utils.Debug(fmt.Sprintf("%v", response))
		} else {
			utils.Error(fmt.Errorf("error in sending SMS: %v", err))
			return
		}

	default:
		utils.Error(fmt.Errorf("invalid channel: %s", data.Channel))
		return
	}

	// After successful processing, delete the message from the queue
	_, err = sqsClient.DeleteMessageWithContext(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.Printf("Failed to delete SQS message: %v", err)
	} else {
		log.Println("Message processed and deleted from SQS.")
	}
}
