package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/config"
	rcs "github.com/wecredit/communication-sdk/internal/channels/rcs"
	sms "github.com/wecredit/communication-sdk/internal/channels/sms"
	"github.com/wecredit/communication-sdk/internal/channels/whatsapp"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/models/awsModels"
	"github.com/wecredit/communication-sdk/internal/redis"
	dbservices "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

type MessageWrapper struct {
	Message *sqs.Message
	Payload sdkModels.CommApiRequestBody
}

type clientRoutine struct {
	msgChan   chan MessageWrapper
	closeOnce sync.Once
	wg        *sync.WaitGroup
	workers   int
}

var (
	clientHandlers           = make(map[string]*clientRoutine)
	clientMux                sync.Mutex
	defaultClientWorkerCount = 5
)

func ConsumerService(workerCount int, queueURL string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleShutdown(cancel)

	for {
		select {
		case <-ctx.Done():
			utils.Warn("Context cancelled. Shutting down all client handlers.")
			clientMux.Lock()
			for client, handler := range clientHandlers {
				handler.closeOnce.Do(func() {
					close(handler.msgChan)
				})
				handler.wg.Wait()
				utils.Info(fmt.Sprintf("Gracefully shut down handler for client: %s", client))
			}
			clientMux.Unlock()
			return
		default:
			result, err := queue.SQSClient.ReceiveMessage(&sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queueURL),
				MaxNumberOfMessages: aws.Int64(10),
				WaitTimeSeconds:     aws.Int64(10),
				VisibilityTimeout:   aws.Int64(15),
			})
			if err != nil {
				utils.Error(fmt.Errorf("error receiving messages: %v", err))
				continue
			}

			for _, msg := range result.Messages {
				go routeMessageToClient(ctx, msg, queueURL)
			}
		}
	}
}

func routeMessageToClient(ctx context.Context, msg *sqs.Message, queueURL string) {
	defer func() {
		if r := recover(); r != nil {
			utils.Error(fmt.Errorf("panic recovered in routeMessageToClient: %v", r))
		}
	}()

	var snsWrapper awsModels.SnsMessageWrapper
	if err := json.Unmarshal([]byte(*msg.Body), &snsWrapper); err != nil {
		utils.Error(fmt.Errorf("failed to unmarshal SNS wrapper: %v", err))
		return
	}

	var data sdkModels.CommApiRequestBody
	if err := json.Unmarshal([]byte(snsWrapper.Message), &data); err != nil {
		utils.Error(fmt.Errorf("failed to unmarshal inner message: %v", err))
		return
	}

	client := strings.ToLower(data.Client)
	if client == "" {
		utils.Warn("Empty client found in payload. Skipping.")
		return
	}

	clientMux.Lock()
	handler, exists := clientHandlers[client]
	if !exists {
		handler = &clientRoutine{
			msgChan: make(chan MessageWrapper, 100),
			wg:      &sync.WaitGroup{},
			workers: defaultClientWorkerCount,
		}
		clientHandlers[client] = handler

		for i := 0; i < handler.workers; i++ {
			handler.wg.Add(1)
			go startClientWorker(ctx, client, handler.msgChan, queue.SQSClient, queueURL, handler.wg)
		}
		utils.Info(fmt.Sprintf("Started %d workers for client: %s", handler.workers, client))
	}
	clientMux.Unlock()

	handler.msgChan <- MessageWrapper{Message: msg, Payload: data}
}

func startClientWorker(ctx context.Context, client string, msgChan <-chan MessageWrapper, sqsClient *sqs.SQS, queueURL string, wg *sync.WaitGroup) {
	defer func() {
		if r := recover(); r != nil {
			utils.Error(fmt.Errorf("panic recovered in client worker [%s]: %v", client, r))
		}
		wg.Done()
	}()

	timeout := time.NewTimer(time.Hour)
	for {
		select {
		case <-ctx.Done():
			utils.Warn(fmt.Sprintf("Shutting down worker for client: %s", client))
			return
		case msgWrapper, ok := <-msgChan:
			if !ok {
				utils.Warn(fmt.Sprintf("Channel closed for client: %s", client))
				return
			}
			if !timeout.Stop() {
				<-timeout.C
			}
			timeout.Reset(time.Hour)
			processMessage(ctx, sqsClient, queueURL, msgWrapper)
		case <-timeout.C:
			utils.Warn(fmt.Sprintf("Worker timeout: no messages for 1 hour for client: %s", client))
			clientMux.Lock()
			if handler, ok := clientHandlers[client]; ok {
				handler.closeOnce.Do(func() {
					close(handler.msgChan)
				})
				delete(clientHandlers, client)
			}
			clientMux.Unlock()
			return
		}
	}
}

func handleShutdown(cancelFunc context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	utils.Warn(fmt.Sprintf("Received shutdown signal: %v", sig))
	cancelFunc()
}

func processMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msgWrapper MessageWrapper) {
	msg := msgWrapper.Message
	data := msgWrapper.Payload

	utils.Debug(fmt.Sprintf("Payload: %+v", data))

	data.Client = strings.ToLower(data.Client)
	data.Channel = strings.ToUpper(data.Channel)
	data.ProcessName = strings.ToUpper(data.ProcessName)

	dbMappedData, err := dbservices.MapIntoDbModel(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	utils.Debug(fmt.Sprintf("[Client:%s CommId:%s] Processing %s", data.Client, data.CommId, data.Channel))

	switch data.Channel {
	case variables.WhatsApp:
		handleWhatsapp(ctx, data, dbMappedData, sqsClient, queueURL, msg)
	case variables.RCS:
		handleRCS(ctx, data, dbMappedData, sqsClient, queueURL, msg)
	case variables.SMS:
		handleSMS(ctx, data, dbMappedData, sqsClient, queueURL, msg)
	default:
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] invalid channel: %s", data.Client, data.CommId, data.Channel))
	}
}

func handleWhatsapp(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) {
	if err := database.InsertData(config.Configs.SdkWhatsappInputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}

	maxCountInt, _ := strconv.Atoi(config.Configs.CreditSeaWhatsappMaxCount)
	if data.Client == variables.CreditSea {
		count, err := redis.GetCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount)
		if err != nil {
			utils.Error(fmt.Errorf("Redis error: %v. Falling back to default vendor."))
		}
		data.Vendor = variables.SINCH
		if count > maxCountInt {
			utils.Error(fmt.Errorf("CreditSea Whatsapp count exceeded: current count:%d, maxCount:%d", count, maxCountInt))
			if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, map[string]interface{}{
				"CommId":          data.CommId,
				"Vendor":          data.Vendor,
				"MobileNumber":    data.Mobile,
				"IsSent":          false,
				"ResponseMessage": fmt.Sprintf("CreditSea whatsapp limit exceeeded. Message not sent for commid: %s", data.CommId),
			}); err != nil {
				utils.Error(fmt.Errorf("error inserting data into table: %v", err))
			}
			deleteMessage(ctx, sqsClient, queueURL, msg, data)
			return
		}
	} else {
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
	}
	response, err := whatsapp.SendWpByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in sending whatsapp: %v", err))
		return
	}
	utils.Debug(fmt.Sprintf("%v", response))

	deleteMessage(ctx, sqsClient, queueURL, msg, data)
}

func handleRCS(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) {
	if err := database.InsertData(config.Configs.SdkRcsInputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}
	AssignVendor(&data)
	response, err := rcs.SendRcsByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending RCS: %v", data.Client, data.CommId, err))
		return
	}
	utils.Debug(fmt.Sprintf("%v", response))
	deleteMessage(ctx, sqsClient, queueURL, msg, data)
}

func handleSMS(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) {
	if err := database.InsertData(config.Configs.SdkSmsInputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}
	AssignVendor(&data)
	_, err := sms.SendSmsByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending SMS: %v", data.Client, data.CommId, err))
		return
	}
	deleteMessage(ctx, sqsClient, queueURL, msg, data)
}

func deleteMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, data sdkModels.CommApiRequestBody) {
	_, err := sqsClient.DeleteMessageWithContext(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] failed to delete SQS message: %v", data.Client, data.CommId, err))
	} else {
		utils.Info(fmt.Sprintf("[Client:%s CommId:%s] Message processed and deleted from SQS.", data.Client, data.CommId))
	}
}

func AssignVendor(data *sdkModels.CommApiRequestBody) {
	if data.Client == variables.CreditSea {
		data.Vendor = variables.SINCH
	} else {
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
	}
}
