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

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
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
	Message  *sqs.Message
	Payload  sdkModels.CommApiRequestBody
	QueueURL string
}

type clientRoutine struct {
	msgChan   chan MessageWrapper
	closeOnce sync.Once
	wg        *sync.WaitGroup
	workers   int
}

var (
	clientHandlers = make(map[string]*clientRoutine)
	clientMux      sync.Mutex
)

const (
	defaultClientWorkers = 5
	defaultClientBuffer  = 100
	maxClientWorkers     = 500
	maxClientBuffer      = 10000
)

func ConsumerQueueURLs() []string {
	seen := make(map[string]struct{})
	urls := make([]string, 0, 2)
	add := func(raw string) {
		u := strings.TrimSpace(raw)
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}
	// Legacy SNS→SQS priority/non-priority subscription queue.
	add(config.Configs.AwsQueueUrl)
	// WeCredit SMS SQS-direct publish target (validate-client + Send).
	add(config.Configs.AwsWeCreditSmsQueueUrl)
	return urls
}

// ConsumerService long-polls every configured SDK work queue and routes into shared
// per-client worker pools. Each work item carries its originating queue URL so
// DeleteMessage targets the queue the message was received from.
func ConsumerService(_ string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleShutdown(cancel)

	queueURLs := ConsumerQueueURLs()
	if len(queueURLs) == 0 {
		utils.Error(fmt.Errorf("no SQS queue URLs configured (set AWS_QUEUE_URL and/or AWS_WECREDIT_SMS_QUEUE_URL)"))
		return
	}
	for _, queueURL := range queueURLs {
		url := queueURL
		utils.Info(fmt.Sprintf("starting communication SQS consumer for queue: %s", url))
		go pollCommunicationQueue(ctx, url)
	}

	<-ctx.Done()
	utils.Warn("Context cancelled. Shutting down all client handlers.")
	clientMux.Lock()
	handlers := make([]*clientRoutine, 0, len(clientHandlers))
	clients := make([]string, 0, len(clientHandlers))
	for client, handler := range clientHandlers {
		handler.closeOnce.Do(func() {
			close(handler.msgChan)
		})
		handlers = append(handlers, handler)
		clients = append(clients, client)
	}
	clientMux.Unlock()
	for i, handler := range handlers {
		handler.wg.Wait()
		utils.Info(fmt.Sprintf("Gracefully shut down handler for client: %s", clients[i]))
	}
}

func pollCommunicationQueue(ctx context.Context, queueURL string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			result, err := queue.SQSClient.ReceiveMessage(&sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queueURL),
				MaxNumberOfMessages: aws.Int64(10),
				WaitTimeSeconds:     aws.Int64(10),
				VisibilityTimeout:   aws.Int64(300),
			})
			if err != nil {
				utils.Error(fmt.Errorf("error receiving messages from %s: %v", queueURL, err))
				continue
			}

			if len(result.Messages) == 0 {
				continue
			}

			utils.Debug(fmt.Sprintf("[Consumer] Received %d messages from queue %s", len(result.Messages), queueURL))

			for _, msg := range result.Messages {
				routeMessageToClient(ctx, msg, queueURL)
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

	data, err := parseSQSCommPayload(*msg.Body)
	if err != nil {
		utils.Error(fmt.Errorf("failed to parse SQS message body: %v", err))
		if _, delErr := deleteMessage(ctx, queue.SQSClient, queueURL, msg, sdkModels.CommApiRequestBody{}); delErr != nil {
			utils.Error(fmt.Errorf("failed to delete unparseable SQS message: %v", delErr))
		}
		return
	}

	client := strings.ToLower(data.Client)
	if client == "" {
		utils.Warn("Empty client found in payload. Skipping.")
		if _, delErr := deleteMessage(ctx, queue.SQSClient, queueURL, msg, data); delErr != nil {
			utils.Error(fmt.Errorf("failed to delete SQS message with empty client: %v", delErr))
		}
		return
	}

	clientMux.Lock()
	handler, exists := clientHandlers[client]
	if !exists {
		workerCount := ClientWorkerCount(client)
		bufferSize := boundedConsumerConfigInt(config.Configs.ConsumerClientBufferSize, defaultClientBuffer, maxClientBuffer)
		handler = &clientRoutine{
			msgChan: make(chan MessageWrapper, bufferSize),
			wg:      &sync.WaitGroup{},
			workers: workerCount,
		}
		clientHandlers[client] = handler

		for i := 0; i < handler.workers; i++ {
			handler.wg.Add(1)
			go startClientWorker(ctx, client, handler.msgChan, queue.SQSClient, handler.wg)
		}
		utils.Info(fmt.Sprintf("Started %d workers for client: %s", handler.workers, client))
	}
	clientMux.Unlock()

	handler.msgChan <- MessageWrapper{Message: msg, Payload: data, QueueURL: queueURL}
}

// parseSQSCommPayload accepts SNS→SQS envelopes (legacy) or raw CommApiRequestBody JSON (SQS-direct).
func parseSQSCommPayload(body string) (sdkModels.CommApiRequestBody, error) {
	var data sdkModels.CommApiRequestBody

	var snsWrapper awsModels.SnsMessageWrapper
	if err := json.Unmarshal([]byte(body), &snsWrapper); err == nil && strings.TrimSpace(snsWrapper.Message) != "" {
		if err := json.Unmarshal([]byte(snsWrapper.Message), &data); err != nil {
			return data, fmt.Errorf("inner SNS message: %w", err)
		}
		return data, nil
	}

	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return data, err
	}
	if strings.TrimSpace(data.Client) == "" && strings.TrimSpace(data.Channel) == "" {
		return data, fmt.Errorf("unrecognized SQS body (not SNS envelope or CommApiRequestBody)")
	}
	return data, nil
}

func ClientWorkerCount(client string) int {
	workers := boundedConsumerConfigInt(config.Configs.ConsumerDefaultClientWorkers, defaultClientWorkers, maxClientWorkers)
	client = strings.ToLower(strings.TrimSpace(client))
	for _, entry := range strings.Split(config.Configs.ConsumerClientWorkerOverrides, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), ":", 2)
		if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), client) {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err == nil && value > 0 && value <= maxClientWorkers {
			return value
		}
	}
	return workers
}

func boundedConsumerConfigInt(raw string, fallback, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func startClientWorker(ctx context.Context, client string, msgChan <-chan MessageWrapper, sqsClient *sqs.SQS, wg *sync.WaitGroup) {
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
			isMessageProcessed, deleted := processMessage(ctx, sqsClient, msgWrapper.QueueURL, msgWrapper)
			// Note: Message deletion is handled inside processMessage and channel handlers
			// Only delete here if processMessage explicitly indicates it should be deleted
			// but wasn't already deleted (e.g., on fatal errors)
			if !isMessageProcessed {
				// If message processing failed and wasn't deleted, we need to decide:
				// - If it's a transient error, don't delete (let it retry)
				// - If it's a permanent error, delete to prevent infinite retries
				// For now, we let SQS handle retries via visibility timeout
				utils.Debug(fmt.Sprintf("[Client:%s] Message processing returned false, will retry after visibility timeout", client))
			} else if isMessageProcessed && !deleted {
				deleted, err := deleteMessage(ctx, sqsClient, msgWrapper.QueueURL, msgWrapper.Message, msgWrapper.Payload)
				if !deleted {
					utils.Error(fmt.Errorf("failed to delete message after processing failed: %v", err))
				}
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

func processMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msgWrapper MessageWrapper) (bool, bool) {
	msg := msgWrapper.Message
	data := msgWrapper.Payload

	// redis check : mobile_channel already exists. no api hit. upsert query.  redis key : mobile_channel -> sinch_responseid -> database insert.
	// continue
	// else store key in redis (mobile_channel)

	// Convert stage to string for Redis key
	// redisKey := channelHelper.GenerateRedisKey(data.Mobile, data.Channel, data.Stage)

	// // check if message already sent for once
	// exists, transactionId, errorMessage, err := redis.GetMobileDataFromRedis(config.Configs.CommIdempotentKey, redisKey, redis.RDB)
	// if err != nil {
	// 	utils.Error(fmt.Errorf("error in checking mobile: %s, redisKey: %s on redis: %v", data.Mobile, redisKey, err))
	// 	// Redis error is transient - don't delete message, let it retry
	// 	return false, false // message is not processed as redis check failed
	// }

	// // If we have data from Redis, handle accordingly
	// if exists {
	// 	// Priority: If we have a transactionId, the message was successfully processed before
	// 	if transactionId != "" {
	// 		// check if record already exists in output table
	// 		dataExistsAlready, err := CheckIfDataAlreadyExists(data, redisKey, transactionId)
	// 		if err != nil {
	// 			utils.Error(fmt.Errorf("error checking if data exists for mobile: %s, redisKey: %s, transactionId: %s: %v", data.Mobile, redisKey, transactionId, err))
	// 			return false, false
	// 		}

	// 		// for debugging purpose
	// 		if dataExistsAlready {
	// 			utils.Debug("Data already exists in output table, skipping processing")
	// 		} else {
	// 			utils.Debug("Data does not exist in output table, inserted new record")
	// 		}
	// 		deleted, err := deleteMessage(ctx, sqsClient, queueURL, msg, data)
	// 		if !deleted {
	// 			utils.Error(fmt.Errorf("failed to delete message after successful processing check: %v", err))
	// 		}
	// 		return true, deleted // message processed
	// 	}

	// 	// If we have an error message (and no transactionId), skip processing
	// 	if errorMessage != "" && transactionId == "" {
	// 		utils.Debug(fmt.Sprintf("Message already processed for redisKey: %s with error: %s, skipping", redisKey, errorMessage))
	// 		deleted, err := deleteMessage(ctx, sqsClient, queueURL, msg, data)
	// 		if !deleted {
	// 			utils.Error(fmt.Errorf("failed to delete message after error processing check: %v", err))
	// 		}
	// 		return true, deleted // message processed
	// 	}

	// 	// Redis key exists but no transactionId or errorMessage - new message
	// 	// Delete it to prevent reprocessing
	// 	// utils.Debug(fmt.Sprintf("Message already processed for redisKey: %s (key exists but no transactionId/errorMessage), deleting", redisKey))
	// 	// deleteMessage(ctx, sqsClient, queueURL, msg, data)
	// 	// return true // message processed
	// }

	// // If not exists, add key with blank value
	// err = redis.SetMobileChannelKey(redis.RDB, config.Configs.CommIdempotentKey, redisKey)
	// if err != nil {
	// 	utils.Error(fmt.Errorf("redis add failed: %v", err))
	// }

	utils.Debug(fmt.Sprintf("Payload: %+v", data))

	data.Client = strings.ToLower(data.Client)
	data.Channel = strings.ToUpper(data.Channel)
	data.ProcessName = strings.ToUpper(data.ProcessName)
	data.AzureIdempotencyKey = fmt.Sprintf("%s_%s", strings.ToLower(data.ProcessName), strings.ToLower(data.Description))

	dbMappedData, err := dbservices.MapIntoDbModel(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
		// Data mapping error is likely permanent - delete message to prevent infinite retries
		// But log it for investigation
		deleted, delErr := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete message after mapping error: %v", delErr))
		}
		return false, deleted // Return false to indicate processing failed
	}

	utils.Debug(fmt.Sprintf("[Client:%s CommId:%s] Processing %s", data.Client, data.CommId, data.Channel))

	switch data.Channel {
	case variables.WhatsApp:
		isMessageProcessed, deleted := handleWhatsapp(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed, deleted
	case variables.RCS:
		isMessageProcessed, deleted := handleRCS(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed, deleted
	case variables.SMS:
		isMessageProcessed, deleted := handleSMS(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed, deleted
	case variables.Email:
		isMessageProcessed, deleted := handleEmail(ctx, data, dbMappedData, sqsClient, queueURL, msg)
		return isMessageProcessed, deleted
	default:
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] invalid channel: %s", data.Client, data.CommId, data.Channel))
		// Delete invalid messages to prevent unnecessary retries
		deleted, err := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete message with invalid channel: %v", err))
		}
		return true, deleted // message processed (rejected due to invalid channel)
	}
}

func handleWhatsapp(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	// if err := database.InsertData(config.Configs.SdkWhatsappInputTable, database.DBtechWrite, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into wp input table for mobile %s: %v", data.Mobile, err))
	// }

	maxCountInt, _ := strconv.Atoi(config.Configs.CreditSeaWhatsappMaxCount)
	if data.Client == variables.CreditSea {
		count, err := redis.GetCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount)
		if err != nil {
			utils.Error(fmt.Errorf("redis error: %v", err))
		}
		if strings.TrimSpace(data.Vendor) == "" {
			data.Vendor = variables.SINCH
		} else if !AssignVendor(&data) {
			return rejectRequestedVendor(ctx, data, sqsClient, queueURL, msg)
		}
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
			}
			deleted, err := deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete message after CreditSea limit exceeded: %v", err))
			}
			return true, deleted // message processed but not sent as CreditSea whatsapp limit exceeeded
		}
	} else {
		if !AssignVendor(&data) {
			return rejectRequestedVendor(ctx, data, sqsClient, queueURL, msg)
		}
	}
	var deleted bool
	var delErr error

	isMessageProcessed, dbMappedData, err := whatsapp.SendWpByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in sending whatsapp: %v", err))
		// If processing failed, don't delete message - let it retry after visibility timeout
		// However, if isMessageProcessed is true (partial success), we should delete to prevent duplicates

		if isMessageProcessed {
			deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete message after partial whatsapp processing: %v", delErr))
			}
		}
		return isMessageProcessed, deleted
	}

	// if message processed successfully, delete it and then insert it into database
	if isMessageProcessed {
		deleted, err = deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete message after successful whatsapp processing: %v", err))
		}
	}

	if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into wp output table for mobile %s: %v", data.Mobile, err))
	}

	return isMessageProcessed, deleted

}

func handleRCS(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	// if err := database.InsertData(config.Configs.SdkRcsInputTable, database.DBtechWrite, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }
	var deleted bool
	var delErr error
	if !AssignVendor(&data) {
		return rejectRequestedVendor(ctx, data, sqsClient, queueURL, msg)
	}
	isMessageProcessed, err := rcs.SendRcsByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending RCS: %v", data.Client, data.CommId, err))
		// If processing failed, don't delete message - let it retry after visibility timeout
		// However, if isMessageProcessed is true (partial success), we should delete to prevent duplicates

		if isMessageProcessed {
			deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete message after partial RCS processing: %v", delErr))
			}
		}
		return isMessageProcessed, deleted
	}

	if isMessageProcessed {
		deleted, err := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete message after successful RCS processing: %v", err))
		}
	}

	return isMessageProcessed, deleted
}

func handleSMS(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	var deleted bool
	var delErr error
	if !AssignVendor(&data) {
		return rejectRequestedVendor(ctx, data, sqsClient, queueURL, msg)
	}
	result, err := sms.SendSmsByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending SMS: %v", data.Client, data.CommId, err))
		if result.Processed && result.AckSQS {
			deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete message after partial SMS processing: %v", delErr))
			}
		}
		return result.Processed, deleted
	}

	if result.AckSQS {
		deleted, err = deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete message after SMS processing: %v", err))
		}
	} else {
		utils.Info(fmt.Sprintf("[Client:%s CommId:%s] retaining SQS message for non-terminal SMS outcome", data.Client, data.CommId))
	}

	if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtechWrite, result.DBData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into sms output table for mobile %s: %v", data.Mobile, err))
	}

	return result.Processed, deleted
}

func handleEmail(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	// delete Mobile from dbMappedData and Add Email in it for successful insertion in email input audit table
	// delete(dbMappedData, "Mobile")
	// dbMappedData["Email"] = data.Email
	// if err := database.InsertData(config.Configs.SdkEmailInputTable, database.DBtechWrite, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }
	var deleted bool
	var delErr error
	if !AssignVendor(&data) {
		return rejectRequestedVendor(ctx, data, sqsClient, queueURL, msg)
	}
	isMessageProcessed, dbMappedData, err := email.SendEmailByProcess(data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending Email: %v", data.Client, data.CommId, err))
		// If processing failed, don't delete message - let it retry after visibility timeout
		// However, if isMessageProcessed is true (partial success), we should delete to prevent duplicates
		if isMessageProcessed {
			deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete message after partial Email processing: %v", delErr))
			}
		}
		return isMessageProcessed, deleted
	}

	if isMessageProcessed {
		deleted, err = deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete message after successful Email processing: %v", err))
		}
	}

	delete(dbMappedData, "MobileNumber")
	dbMappedData["Email"] = data.Email

	if err := database.InsertData(config.Configs.EmailOutputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}

	return isMessageProcessed, deleted
}

func deleteMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, data sdkModels.CommApiRequestBody) (bool, error) {
	deleteCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	maxAttempts := 5
	backoff := time.Second

	for i := 1; i <= maxAttempts; i++ {
		// Check if context is cancelled or timed out
		select {
		case <-deleteCtx.Done():
			return false, fmt.Errorf("[Client:%s CommId:%s] context cancelled or timed out while deleting message: %v", data.Client, data.CommId, deleteCtx.Err())
		default:
		}

		_, err := sqsClient.DeleteMessageWithContext(deleteCtx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(queueURL),
			ReceiptHandle: msg.ReceiptHandle,
		})
		if err == nil {
			utils.Info(fmt.Sprintf("[Client:%s CommId:%s] Message successfully deleted from SQS on attempt %d", data.Client, data.CommId, i))
			return true, nil
		}

		// Log error and retry
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] delete attempt %d/%d failed: %v", data.Client, data.CommId, i, maxAttempts, err))

		// If this is not the last attempt, wait before retrying
		if i < maxAttempts {
			select {
			case <-deleteCtx.Done():
				return false, fmt.Errorf("[Client:%s CommId:%s] context cancelled during backoff: %v", data.Client, data.CommId, deleteCtx.Err())
			case <-time.After(backoff):
				backoff *= 2 // Exponential backoff
			}
		}
	}

	// All attempts failed
	err := fmt.Errorf("[Client:%s CommId:%s] failed to delete SQS message after %d attempts", data.Client, data.CommId, maxAttempts)
	utils.Error(err)
	return false, err
}

func AssignVendor(data *sdkModels.CommApiRequestBody) bool {
	requestedVendor := strings.ToUpper(strings.TrimSpace(data.Vendor))
	if requestedVendor != "" {
		data.Vendor = requestedVendor
		if channelHelper.IsVendorActive(data.Client, requestedVendor, data.Channel) {
			utils.Debug(fmt.Sprintf("Preserved requested vendor: %s for client: %s, channel: %s, commId: %s", data.Vendor, data.Client, data.Channel, data.CommId))
			return true
		}
		utils.Error(fmt.Errorf("requested vendor is not active for client: %s, channel: %s, commId: %s", data.Client, data.Channel, data.CommId))
		return false
	}

	if data.Client == variables.CreditSea || data.Channel == variables.Email {
		data.Vendor = variables.SINCH
	} else {
		data.Vendor = GetVendorByClientAndChannel(data.Channel, data.Client, data.CommId)
		utils.Debug(fmt.Sprintf("Assigned vendor: %s for client: %s, channel: %s, commId: %s", data.Vendor, data.Client, data.Channel, data.CommId))
	}
	return true
}

func rejectRequestedVendor(ctx context.Context, data sdkModels.CommApiRequestBody, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	deleted, err := deleteMessage(ctx, sqsClient, queueURL, msg, data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to delete message rejected for inactive requested vendor: %v", err))
	}
	return true, deleted
}
