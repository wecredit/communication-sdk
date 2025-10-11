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
	email "github.com/wecredit/communication-sdk/internal/channels/email"
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
				VisibilityTimeout:   aws.Int64(300),
			})
			if err != nil {
				utils.Error(fmt.Errorf("error receiving messages: %v", err))
				continue
			}

			if len(result.Messages) == 0 {
				continue
			}

			utils.Debug(fmt.Sprintf("[Consumer] Received %d messages from queue %s", len(result.Messages), queueURL))

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
			isMessageProcessed := processMessage(ctx, sqsClient, queueURL, msgWrapper)
			if isMessageProcessed {
				utils.Debug("Message processed")
				// deleteMessage(ctx, sqsClient, queueURL, msgWrapper.Message, msgWrapper.Payload) // delete message from SQS if it is processed
			}
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

func processMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msgWrapper MessageWrapper) bool {
	msg := msgWrapper.Message
	data := msgWrapper.Payload

	// redis check : mobile_channel already exists. no api hit. upsert query.  redis key : mobile_channel -> sinch_responseid -> database insert.
	// continue
	// else store key in redis (mobile_channel)

	// redis fails, project close

	redisKey := fmt.Sprintf("%s_%s", data.Mobile, strings.ToUpper(data.Channel))

	// check if message already sent for once
	transactionId, exists, err := redis.CheckIfMobileExists(config.Configs.CommIdempotentKey, redisKey, redis.RDB)
	if err != nil {
		utils.Error(fmt.Errorf("error in checking mobile on redis: %v", err))
	}

	if exists {
		// check for transactionId stored in the rediskey, if exists -> save in database, else -> send message to error queue.
		if transactionId != "" {
			// check if record already exists in output table
			dataExistsAlready, err := CheckIfDataAlreadyExists(data, redisKey, transactionId)
			if err != nil {
				utils.Error(fmt.Errorf("error checking if data exists: %v", err))
				return false
			}

			// for debugging purpose
			if dataExistsAlready {
				utils.Debug("Data already exists in output table, skipping processing")
			} else {
				utils.Debug("Data does not exist in output table, inserted new record")
			}
		} else {
			if queueErr := queue.SendMessageWithSubject(sqsClient, msg, config.Configs.AwsErrorQueueUrl, variables.RedisValueMissing, ""); queueErr != nil {
				utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
			}
		}

		deleteMessage(ctx, sqsClient, queueURL, msg, data)
		return true // message processed
	}

	// If not exists, add key with blank value
	err = redis.SetMobileChannelKey(redis.RDB, config.Configs.CommIdempotentKey, redisKey)
	if err != nil {
		utils.Error(fmt.Errorf("redis add failed: %v", err))
	}

	utils.Debug(fmt.Sprintf("Payload: %+v", data))

	data.Client = strings.ToLower(data.Client)
	data.Channel = strings.ToUpper(data.Channel)
	data.ProcessName = strings.ToUpper(data.ProcessName)
	data.AzureIdempotencyKey = fmt.Sprintf("%s_%s", strings.ToLower(data.ProcessName), strings.ToLower(data.Description))

	dbMappedData, err := dbservices.MapIntoDbModel(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	utils.Debug(fmt.Sprintf("[Client:%s CommId:%s] Processing %s", data.Client, data.CommId, data.Channel))

	switch data.Channel {
	case variables.WhatsApp:
		isMessageProcessed := handleWhatsapp(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed
	case variables.RCS:
		isMessageProcessed := handleRCS(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed
	case variables.SMS:
		isMessageProcessed := handleSMS(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed
	case variables.Email:
		isMessageProcessed := handleEmail(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed
	default:
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] invalid channel: %s", data.Client, data.CommId, data.Channel))
		return false
	}
}

func handleWhatsapp(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) bool {
	if err := database.InsertData(config.Configs.SdkWhatsappInputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into wp input table for mobile %s: %v", data.Mobile, err))
		dbMappedData["tableName"] = config.Configs.SdkWhatsappInputTable
		if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.InputInsertionFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
	}

	maxCountInt, _ := strconv.Atoi(config.Configs.CreditSeaWhatsappMaxCount)
	if data.Client == variables.CreditSea {
		count, err := redis.GetCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount)
		if err != nil {
			utils.Error(fmt.Errorf("redis error: %v", err))
		}
		data.Vendor = variables.SINCH
		if count > maxCountInt {
			utils.Error(fmt.Errorf("CreditSea Whatsapp count exceeded: current count:%d, maxCount:%d", count, maxCountInt))
			if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtechWrite, map[string]interface{}{
				"CommId":          data.CommId,
				"Vendor":          data.Vendor,
				"MobileNumber":    data.Mobile,
				"IsSent":          false,
				"ResponseMessage": fmt.Sprintf("CreditSea whatsapp limit exceeeded. Message not sent for commid: %s", data.CommId),
			}); err != nil {
				utils.Error(fmt.Errorf("error inserting data into wp output table for mobile %s: %v", data.Mobile, err))
				dbMappedData["tableName"] = config.Configs.WhatsappOutputTable
				if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.OutputInsertionFails, err.Error()); queueErr != nil {
					utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
				}
			}
			deleteMessage(ctx, sqsClient, queueURL, msg, data)
			return true // message processed but not sent as CreditSea whatsapp limit exceeeded
		}
	} else {
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
	}

	isMessageProcessed, dbMappedData, err := whatsapp.SendWpByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in sending whatsapp: %v", err))
		return isMessageProcessed
	}

	// if message processed successfully, delete it and then insert it into database
	if isMessageProcessed {
		deleteMessage(ctx, sqsClient, queueURL, msg, data)
	}

	if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into wp output table for mobile %s: %v", data.Mobile, err))
		dbMappedData["tableName"] = config.Configs.WhatsappOutputTable
		if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.OutputInsertionFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
	}

	return isMessageProcessed

}

func handleRCS(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) bool {
	if err := database.InsertData(config.Configs.SdkRcsInputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		dbMappedData["tableName"] = config.Configs.SdkRcsInputTable
		if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.InputInsertionFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
	}
	AssignVendor(&data)
	isMessageProcessed, err := rcs.SendRcsByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending RCS: %v", data.Client, data.CommId, err))
		return isMessageProcessed
	}
	deleteMessage(ctx, sqsClient, queueURL, msg, data)
	return isMessageProcessed
}

func handleSMS(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) bool {
	if err := database.InsertData(config.Configs.SdkSmsInputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into sms input table for mobile %s: %v", data.Mobile, err))
		dbMappedData["tableName"] = config.Configs.SdkSmsInputTable
		if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.InputInsertionFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
	}
	AssignVendor(&data)
	isMessageProcessed, dbMappedData, err := sms.SendSmsByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending SMS: %v", data.Client, data.CommId, err))
		return isMessageProcessed
	}

	if isMessageProcessed {
		deleteMessage(ctx, sqsClient, queueURL, msg, data)
	}

	if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into sms output table for mobile %s: %v", data.Mobile, err))
		dbMappedData["tableName"] = config.Configs.SmsOutputTable
		if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.OutputInsertionFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
	}

	return isMessageProcessed
}

func handleEmail(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) bool {
	// delete Mobile from dbMappedData and Add Email in it for successful insertion in email input audit table
	delete(dbMappedData, "Mobile")
	dbMappedData["Email"] = data.Email
	if err := database.InsertData(config.Configs.SdkEmailInputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
		dbMappedData["tableName"] = config.Configs.SdkEmailInputTable
		if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.InputInsertionFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
	}
	AssignVendor(&data)
	isMessageProcessed, dbMappedData, err := email.SendEmailByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending SMS: %v", data.Client, data.CommId, err))
		return isMessageProcessed
	}

	if isMessageProcessed {
		deleteMessage(ctx, sqsClient, queueURL, msg, data)
	}

	if err := database.InsertData(config.Configs.EmailOutputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table for mobile %s: %v", data.Mobile, err))
		dbMappedData["tableName"] = config.Configs.EmailOutputTable
		if queueErr := queue.SendMessageWithSubject(sqsClient, dbMappedData, config.Configs.AwsErrorQueueUrl, variables.OutputInsertionFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
	}

	return isMessageProcessed
}

func deleteMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, data sdkModels.CommApiRequestBody) {
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	maxAttempts := 3
	backoff := time.Second

	for i := 1; i <= maxAttempts; i++ {
		_, err := sqsClient.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      aws.String(queueURL),
			ReceiptHandle: msg.ReceiptHandle,
		})
		if err == nil {
			utils.Info(fmt.Sprintf("[Client:%s CommId:%s] Message deleted from SQS.", data.Client, data.CommId))
			return
		}
		// Inspect error - if it's an invalid receipt handle, no point retrying
		if strings.Contains(err.Error(), "InvalidReceiptHandle") {
			utils.Warn(fmt.Sprintf("[Client:%s CommId:%s] InvalidReceiptHandle while deleting message: %v", data.Client, data.CommId, err))
			return
		}
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] delete attempt %d failed: %v", data.Client, data.CommId, i, err))
		time.Sleep(backoff)
		backoff *= 2
	}
	utils.Error(fmt.Errorf("[Client:%s CommId:%s] failed to delete SQS message after %d attempts",
		data.Client, data.CommId, maxAttempts))
}

func AssignVendor(data *sdkModels.CommApiRequestBody) {
	if data.Client == variables.CreditSea || data.Channel == variables.Email {
		data.Vendor = variables.SINCH
	} else {
		data.Vendor = GetVendorByChannel(data.Channel, data.CommId)
	}
}
