package services

import (
	"context"
	"encoding/json"
	"errors"
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
	"golang.org/x/sync/errgroup"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	email "github.com/wecredit/communication-sdk/internal/channels/email"
	push "github.com/wecredit/communication-sdk/internal/channels/push"
	rcs "github.com/wecredit/communication-sdk/internal/channels/rcs"
	sms "github.com/wecredit/communication-sdk/internal/channels/sms"
	"github.com/wecredit/communication-sdk/internal/channels/whatsapp"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/metrics"
	"github.com/wecredit/communication-sdk/internal/models/awsModels"
	"github.com/wecredit/communication-sdk/internal/redis"
	dbservices "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/internal/services/monitoring"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	sdkServices "github.com/wecredit/communication-sdk/sdk/services"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

type MessageWrapper struct {
	Message                *sqs.Message
	Payload                sdkModels.CommApiRequestBody
	QueueURL               string
	RedriveMaxReceiveCount int
}

type sqsQueueAttributesAPI interface {
	GetQueueAttributes(*sqs.GetQueueAttributesInput) (*sqs.GetQueueAttributesOutput, error)
}

type sqsRedrivePolicy struct {
	DeadLetterTargetARN string          `json:"deadLetterTargetArn"`
	MaxReceiveCount     json.RawMessage `json:"maxReceiveCount"`
}

type ConsumerQueueRuntime struct {
	URL                    string
	RedriveMaxReceiveCount int
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
	urls := make([]string, 0, 3)
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
	// WeCredit + TrustFin WhatsApp SQS-direct staging target.
	add(config.Configs.AwsWeCreditWhatsappQueueUrl)
	return urls
}

func LoadWhatsappRedriveMaxReceiveCount(client sqsQueueAttributesAPI, queueURL string) (int, error) {
	if client == nil {
		return 0, fmt.Errorf("SQS client is not initialized")
	}

	queueURL = strings.TrimSpace(queueURL)
	if queueURL == "" {
		return 0, fmt.Errorf("WhatsApp queue URL is required")
	}

	result, err := client.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []*string{aws.String("RedrivePolicy")},
	})

	if err != nil {
		return 0, fmt.Errorf("get WhatsApp queue redrive policy: %w", err)
	}

	if result == nil || result.Attributes == nil {
		return 0, fmt.Errorf("WhatsApp queue redrive policy is missing")
	}

	return ParseRedriveMaxReceiveCount(aws.StringValue(result.Attributes["RedrivePolicy"]))
}

func ParseRedriveMaxReceiveCount(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("WhatsApp queue redrive policy is missing")
	}

	var policy sqsRedrivePolicy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return 0, fmt.Errorf("parse WhatsApp queue redrive policy: %w", err)
	}

	if strings.TrimSpace(policy.DeadLetterTargetARN) == "" {
		return 0, fmt.Errorf("WhatsApp queue redrive policy has no dead-letter target")
	}

	maxReceiveCount, err := parseRedriveMaxReceiveCountValue(policy.MaxReceiveCount)
	if err != nil || maxReceiveCount < 1 {
		return 0, fmt.Errorf("WhatsApp queue redrive policy has invalid maxReceiveCount")
	}

	return maxReceiveCount, nil
}

// parseRedriveMaxReceiveCountValue accepts AWS RedrivePolicy shapes where
// maxReceiveCount is either a JSON number (5) or a JSON string ("5").
func parseRedriveMaxReceiveCountValue(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("missing maxReceiveCount")
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, fmt.Errorf("missing maxReceiveCount")
	}

	if trimmed[0] == '"' {
		var asString string
		if err := json.Unmarshal(raw, &asString); err != nil {
			return 0, err
		}
		return strconv.Atoi(strings.TrimSpace(asString))
	}

	var asNumber int
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return 0, err
	}

	return asNumber, nil
}

// PrepareConsumerQueues validates queue URLs and redrive policies for WhatsApp.
func PrepareConsumerQueues(client sqsQueueAttributesAPI, queueURLs []string, whatsappQueueURL string) []ConsumerQueueRuntime {
	runtimes := make([]ConsumerQueueRuntime, 0, len(queueURLs))
	whatsappQueueURL = strings.TrimSpace(whatsappQueueURL)
	for _, rawURL := range queueURLs {
		url := strings.TrimSpace(rawURL)
		if url == "" {
			continue
		}

		runtime := ConsumerQueueRuntime{URL: url}
		if whatsappQueueURL != "" && strings.EqualFold(url, whatsappQueueURL) {
			maxReceiveCount, err := LoadWhatsappRedriveMaxReceiveCount(client, url)
			if err != nil {
				utils.Error(fmt.Errorf("WhatsApp consumer disabled: %v", err))
				metrics.CountByReason("MarketingWhatsappQueueConfigError", "wecredit-whatsapp", "invalid_redrive_policy", 1)
				continue
			}

			runtime.RedriveMaxReceiveCount = maxReceiveCount
			utils.Info(fmt.Sprintf("validated WhatsApp SQS redrive policy maxReceiveCount=%d", maxReceiveCount))
		}

		runtimes = append(runtimes, runtime)
	}

	return runtimes
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
	for _, runtime := range PrepareConsumerQueues(queue.SQSClient, queueURLs, config.Configs.AwsWeCreditWhatsappQueueUrl) {
		url := runtime.URL
		utils.Info(fmt.Sprintf("starting communication SQS consumer for queue: %s", url))
		go pollCommunicationQueue(ctx, url, runtime.RedriveMaxReceiveCount)
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
		delete(clientHandlers, client)
		handlers = append(handlers, handler)
		clients = append(clients, client)
	}
	clientMux.Unlock()
	for i, handler := range handlers {
		handler.wg.Wait()
		utils.Info(fmt.Sprintf("Gracefully shut down handler for client: %s", clients[i]))
	}
}

func pollCommunicationQueue(ctx context.Context, queueURL string, redriveMaxReceiveCount int) {
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
				AttributeNames:      []*string{aws.String("ApproximateReceiveCount"), aws.String("SentTimestamp")},
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
				routeMessageToClient(ctx, msg, queueURL, redriveMaxReceiveCount)
			}
		}
	}
}

func routeMessageToClient(ctx context.Context, msg *sqs.Message, queueURL string, redriveMaxReceiveCount int) {
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

	channel := strings.ToLower(strings.TrimSpace(data.Channel))
	poolKey := ClientChannelPoolKey(client, channel)

	clientMux.Lock()
	handler, exists := clientHandlers[poolKey]
	if !exists {
		workerCount := ClientChannelWorkerCount(client, channel)
		bufferSize := boundedConsumerConfigInt(config.Configs.ConsumerClientBufferSize, defaultClientBuffer, maxClientBuffer)
		handler = &clientRoutine{
			msgChan: make(chan MessageWrapper, bufferSize),
			wg:      &sync.WaitGroup{},
			workers: workerCount,
		}
		clientHandlers[poolKey] = handler

		for i := 0; i < handler.workers; i++ {
			handler.wg.Add(1)
			go startClientWorker(ctx, poolKey, handler.msgChan, queue.SQSClient, handler.wg)
		}
		utils.Info(fmt.Sprintf("Started %d workers for pool: %s", handler.workers, poolKey))
	}
	handler.msgChan <- MessageWrapper{
		Message:                msg,
		Payload:                data,
		QueueURL:               queueURL,
		RedriveMaxReceiveCount: redriveMaxReceiveCount,
	}
	clientMux.Unlock()
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

// ClientChannelPoolKey is client|channel (channel empty → client|default).
func ClientChannelPoolKey(client, channel string) string {
	client = strings.ToLower(strings.TrimSpace(client))
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "" {
		channel = "default"
	}
	return client + "|" + channel
}

// ClientChannelWorkerCount resolves CONSUMER_CHANNEL_WORKER_OVERRIDES
// (client:channel:n), then the broad SMS/WhatsApp channel setting, then
// CONSUMER_CLIENT_WORKER_OVERRIDES (client:n), then default.
func ClientChannelWorkerCount(client, channel string) int {
	client = strings.ToLower(strings.TrimSpace(client))
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel != "" {
		want := client + ":" + channel
		for _, entry := range strings.Split(config.Configs.ConsumerChannelWorkerOverrides, ",") {
			parts := strings.Split(strings.TrimSpace(entry), ":")
			if len(parts) != 3 {
				continue
			}

			key := strings.ToLower(strings.TrimSpace(parts[0])) + ":" + strings.ToLower(strings.TrimSpace(parts[1]))
			if key != want {
				continue
			}

			value, err := strconv.Atoi(strings.TrimSpace(parts[2]))
			if err == nil && value > 0 && value <= maxClientWorkers {
				return value
			}
		}
	}

	var channelWorkers string
	switch channel {
	case strings.ToLower(variables.SMS):
		channelWorkers = config.Configs.SMSWorkers
	case strings.ToLower(variables.WhatsApp):
		channelWorkers = config.Configs.WhatsAppWorkers
	}

	if workers := boundedConsumerConfigInt(channelWorkers, 0, maxClientWorkers); workers > 0 {
		return workers
	}

	return ClientWorkerCount(client)
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

func startClientWorker(ctx context.Context, poolKey string, msgChan <-chan MessageWrapper, sqsClient *sqs.SQS, wg *sync.WaitGroup) {
	defer func() {
		if r := recover(); r != nil {
			utils.Error(fmt.Errorf("panic recovered in client worker [%s]: %v", poolKey, r))
		}
		wg.Done()
	}()

	timeout := time.NewTimer(time.Hour)
	for {
		select {
		case <-ctx.Done():
			utils.Warn(fmt.Sprintf("Shutting down worker for pool: %s", poolKey))
			return
		case msgWrapper, ok := <-msgChan:
			if !ok {
				utils.Warn(fmt.Sprintf("Channel closed for pool: %s", poolKey))
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
				utils.Debug(fmt.Sprintf("[Pool:%s] Message processing returned false, will retry after visibility timeout", poolKey))
			} else if isMessageProcessed && !deleted {
				deleted, err := deleteMessage(ctx, sqsClient, msgWrapper.QueueURL, msgWrapper.Message, msgWrapper.Payload)
				if !deleted {
					utils.Error(fmt.Errorf("failed to delete message after processing failed: %v", err))
				}
			}
		case <-timeout.C:
			utils.Warn(fmt.Sprintf("Worker timeout: no messages for 1 hour for pool: %s", poolKey))
			clientMux.Lock()
			if handler, ok := clientHandlers[poolKey]; ok {
				handler.closeOnce.Do(func() {
					close(handler.msgChan)
				})
				delete(clientHandlers, poolKey)
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

	if strings.EqualFold(data.Channel, variables.PUSH) {
		utils.Debug(fmt.Sprintf("PUSH payload received client=%s commId=%s eventId=%s deviceCount=%d",
			data.Client, data.CommId, data.EventId, len(data.DeviceTokens)))
	} else {
		utils.Debug(fmt.Sprintf("Payload: %+v", data))
	}

	data.Client = strings.ToLower(strings.TrimSpace(data.Client))
	data.Channel = strings.ToUpper(strings.TrimSpace(data.Channel))
	data.ProcessName = strings.ToUpper(strings.TrimSpace(data.ProcessName))
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
		isMessageProcessed, deleted := handleWhatsapp(ctx, data, dbMappedData, sqsClient, queueURL, msg, msgWrapper.RedriveMaxReceiveCount)
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
	case variables.PUSH:
		isMessageProcessed, deleted := handlePush(ctx, data, sqsClient, queueURL, msg)
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

func handlePush(ctx context.Context, data sdkModels.CommApiRequestBody, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	if !AssignVendor(&data) {
		return rejectRequestedVendor(ctx, data, sqsClient, queueURL, msg)
	}

	result, err := push.Send(ctx, data)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s EventId:%s] PUSH processing failed: %w",
			data.Client, data.CommId, data.EventId, err))
	}

	// Persist terminal PUSH audits before ACK. On redelivery push.Send rebuilds
	// terminal output audits from Redis claims without calling FCM again.
	if auditErr := writePushAudits(result); auditErr != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s EventId:%s] PUSH audit persistence failed: %w",
			data.Client, data.CommId, data.EventId, auditErr))
		return false, false
	}

	if !result.AckSQS {
		return result.Processed, false
	}

	deleted, deleteErr := deleteMessage(ctx, sqsClient, queueURL, msg, data)
	if deleteErr != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s EventId:%s] failed to delete terminal PUSH message: %w",
			data.Client, data.CommId, data.EventId, deleteErr))
	}
	return result.Processed, deleted
}

func writePushAudits(result push.Result) error {
	if result.InputAudit != nil {
		if err := database.InsertData(config.Configs.PushInputAuditTable, database.DBtechWrite, result.InputAudit); err != nil && !isDuplicateKeyError(err) {
			return fmt.Errorf("insert input audit: %w", err)
		}
	}
	for _, output := range result.OutputAudits {
		if err := database.InsertData(config.Configs.PushOutputTable, database.DBtechWrite, output); err != nil && !isDuplicateKeyError(err) {
			return fmt.Errorf("insert output audit: %w", err)
		}
	}
	return nil
}

func isDuplicateKeyError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate entry")
}

func handleWhatsapp(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, redriveMaxReceiveCount int) (bool, bool) {
	if isMarketingWPDispatch(data) {
		return handleMarketingWhatsapp(ctx, data, dbMappedData, sqsClient, queueURL, msg, redriveMaxReceiveCount)
	}

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

	wpResult, err := whatsapp.SendWpByProcess(data)
	isMessageProcessed := wpResult.Processed
	dbMappedData = wpResult.DBData
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

	if ShouldSubmitZapCashMonitoring(data, wpResult.Accepted) {
		if !monitoring.TrySubmit(monitoring.AcceptedResult{
			Payload:           data,
			ResolvedVendor:    wpResult.ResolvedVendor,
			ResolvedTemplate:  wpResult.ResolvedTemplate,
			TemplateVariables: wpResult.TemplateVariables,
			TransactionID:     wpResult.TransactionID,
		}) {
			utils.Warn(fmt.Sprintf("[Client:%s CommId:%s] ZapCash monitoring copy dropped after accepted WhatsApp send for vendor=%s template=%s",
				data.Client, data.CommId, wpResult.ResolvedVendor, wpResult.ResolvedTemplate))
		}
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

// MarketingWhatsappDependencies defines the dependencies for marketing WhatsApp dispatch.
// Terminal outcomes write MySQL WhatsappOutputTable + Marketing CommWhatsappMarketingOutput
// (SMS parity: SmsOutputTable + CommDispatchTracking). Not CommDispatchTracking.
type MarketingWhatsappDependencies struct {
	Claim       func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error)
	Assign      func(*sdkModels.CommApiRequestBody) bool
	Send        func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error)
	UpdateError func(sdkModels.CommApiRequestBody, string) error
	// WriteInputAudit persists SdkWhatsappInputTable (best-effort; SMS SdkSmsInput parity).
	WriteInputAudit func(sdkModels.CommApiRequestBody, map[string]interface{}) error
	// WriteOutput persists using the current request payload (post-Assign / ResolveCommID).
	WriteOutput func(sdkModels.CommApiRequestBody, map[string]interface{}) error
	// OutputRecorded checks Marketing output for SourceRowId (Redis skip-send redelivery idempotency).
	OutputRecorded func(int64) (bool, error)
	Delete         func(sdkModels.CommApiRequestBody) (bool, error)
	Release        func(sdkModels.CommApiRequestBody)
	Blank          func(sdkModels.CommApiRequestBody, *sqs.Message, int)
}

// handleMarketingWhatsapp handles marketing WhatsApp dispatch.
func handleMarketingWhatsapp(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, redriveMaxReceiveCount int) (bool, bool) {
	deps := MarketingWhatsappDependencies{
		Claim:  claimOrSkipMarketingDispatch,
		Assign: AssignVendor,
		Send: func(msg sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
			result, err := whatsapp.SendWpByProcess(msg)
			return result.Processed, result.DBData, err
		},
		UpdateError: channelHelper.UpdateRedisErrorMessage,
		WriteInputAudit: func(payload sdkModels.CommApiRequestBody, audit map[string]interface{}) error {
			// SMS marketing inserts SdkSmsInputTable before send; same for WA.
			if err := database.InsertData(config.Configs.SdkWhatsappInputTable, database.DBtechWrite, audit); err != nil {
				utils.Error(fmt.Errorf("[Client:%s CommId:%s] error inserting whatsapp input audit: %v", payload.Client, payload.CommId, err))
			}
			return nil // best-effort; never block the send (SMS parity)
		},
		WriteOutput: func(payload sdkModels.CommApiRequestBody, output map[string]interface{}) error {
			// Dual sink (SMS parity): MySQL WhatsappOutputTable + Marketing Output.
			// Both must succeed before SQS ACK. Lender-only WA still uses MySQL alone
			// in handleWhatsapp.
			mysqlErr, marketingErr := RunParallelSMSPostSendWrites(
				func() error {
					return database.InsertData(
						config.Configs.WhatsappOutputTable,
						database.DBtechWrite,
						MapMarketingWhatsappMysqlOutput(payload, output),
					)
				},
				func() error {
					return database.InsertData(
						config.Configs.CommWhatsappMarketingOutputTable,
						database.DBMarketing,
						MapMarketingWhatsappOutput(payload, output),
					)
				},
			)
			if mysqlErr != nil || marketingErr != nil {
				logWhatsappPostSendPersistenceFailure(payload, mysqlErr, marketingErr)
				if mysqlErr != nil {
					return mysqlErr
				}
				return marketingErr
			}
			return nil
		},
		OutputRecorded: func(sourceRowId int64) (bool, error) {
			return database.CommWhatsappMarketingOutputExists(
				database.DBMarketing,
				config.Configs.CommWhatsappMarketingOutputTable,
				sourceRowId,
			)
		},
		Delete: func(payload sdkModels.CommApiRequestBody) (bool, error) {
			return deleteMessage(ctx, sqsClient, queueURL, msg, payload)
		},
		Release: releaseMarketingDispatchClaims,
		Blank:   recordBlankMarketingWhatsappClaim,
	}

	return HandleMarketingWhatsappWithDependencies(data, dbMappedData, msg, redriveMaxReceiveCount, deps)
}

// writeMarketingWhatsappTerminalOutput persists a terminal WA outcome to both audit sinks, then ACKs SQS.
func writeMarketingWhatsappTerminalOutput(data sdkModels.CommApiRequestBody, deps MarketingWhatsappDependencies, output map[string]interface{}, deleteFailMsg string) (bool, bool) {
	if err := deps.WriteOutput(data, output); err != nil {
		return false, false
	}

	deleted, err := deps.Delete(data)
	if !deleted {
		utils.Error(fmt.Errorf("%s: %v", deleteFailMsg, err))
	}

	return true, deleted
}

// HandleMarketingWhatsappWithDependencies handles marketing WhatsApp dispatch with dependencies.
func HandleMarketingWhatsappWithDependencies(data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, msg *sqs.Message, redriveMaxReceiveCount int, deps MarketingWhatsappDependencies) (bool, bool) {
	skipSend, campaignDuplicate, redisTxn, redisErr, claimErr := deps.Claim(data)
	if claimErr != nil {
		utils.Error(fmt.Errorf("[Client:%s EventId:%s] marketing WhatsApp redis claim failed: %v", data.Client, data.EventId, claimErr))
		return false, false
	}

	if skipSend {
		if strings.TrimSpace(redisTxn) == "" && strings.TrimSpace(redisErr) == "" {
			deps.Blank(data, msg, redriveMaxReceiveCount)
			return false, false
		}

		// Redelivery after a successful terminal write: repair SQS only (SMS tracking idempotency parity).
		if data.SourceRowId != 0 && deps.OutputRecorded != nil {
			alreadyRecorded, existsErr := deps.OutputRecorded(data.SourceRowId)
			if existsErr != nil {
				utils.Error(fmt.Errorf("[Client:%s SourceRowId:%d] marketing WhatsApp output existence check failed: %v", data.Client, data.SourceRowId, existsErr))
				return false, false
			}

			if alreadyRecorded {
				deleted, delErr := deps.Delete(data)
				if !deleted {
					utils.Error(fmt.Errorf("failed to delete redelivered marketing WhatsApp after output already recorded: %v", delErr))
				}
				return true, deleted
			}
		}

		output := map[string]interface{}{}
		if txn := strings.TrimSpace(redisTxn); txn != "" {
			output["IsSent"] = true
			output["TransactionId"] = txn
		} else {
			output["IsSent"] = false
			output["ResponseMessage"] = strings.TrimSpace(redisErr)
		}

		return writeMarketingWhatsappTerminalOutput(data, deps, output, "failed to delete terminal Redis duplicate marketing WhatsApp")
	}

	if campaignDuplicate {
		dupErr := campaignDuplicateError(data)
		// Record a terminal Redis value so redelivery does not see a blank claim.
		if err := deps.UpdateError(data, dupErr); err != nil {
			utils.Error(fmt.Errorf("[Client:%s EventId:%s] failed to record campaign-duplicate WhatsApp in Redis: %v", data.Client, data.EventId, err))
			return false, false
		}
		return writeMarketingWhatsappTerminalOutput(data, deps, map[string]interface{}{
			"IsSent":          false,
			"ResponseMessage": dupErr,
		}, "failed to delete campaign-duplicate marketing WhatsApp")
	}

	data.CommId = sdkServices.ResolveCommID(data.CommId, data.Client)
	if !deps.Assign(&data) {
		const inactiveVendorError = "requested vendor is not active"
		if err := deps.UpdateError(data, inactiveVendorError); err != nil {
			utils.Error(fmt.Errorf("[Client:%s EventId:%s] failed to record inactive WhatsApp vendor in Redis: %v", data.Client, data.EventId, err))
			return false, false
		}

		return writeMarketingWhatsappTerminalOutput(data, deps, map[string]interface{}{
			"IsSent":          false,
			"ResponseMessage": inactiveVendorError,
		}, "failed to delete WhatsApp rejected for inactive vendor")
	}

	// SMS marketing inserts SdkSmsInput before send; WA inserts SdkWhatsappInput the same way.
	if deps.WriteInputAudit != nil {
		if dbMappedData == nil {
			dbMappedData = map[string]interface{}{}
		}
		dbMappedData["CommId"] = data.CommId
		_ = deps.WriteInputAudit(data, dbMappedData)
	}

	// WeCredit WA same-day cap is client_channel_mobile (no process); cleared by 1 AM FlushAll.
	isMessageProcessed, outputData, sendErr := deps.Send(data)
	if sendErr != nil && !isMessageProcessed {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] retryable WhatsApp processing error: %v", data.Client, data.CommId, sendErr))
		deps.Release(data)
		return false, false
	}

	if !isMessageProcessed {
		deps.Release(data)
		return false, false
	}

	if outputData == nil {
		outputData = dbMappedData
	}
	if outputData == nil {
		outputData = map[string]interface{}{}
	}

	responseMessage := mapString(outputData, "ResponseMessage")
	isSent := mapBool(outputData, "IsSent")
	if !isSent {
		if responseMessage == "" && sendErr != nil {
			responseMessage = sendErr.Error()
		}
		if responseMessage == "" {
			responseMessage = "WhatsApp provider rejected the request"
		}
		outputData["ResponseMessage"] = responseMessage
		outputData["IsSent"] = false
		if err := deps.UpdateError(data, responseMessage); err != nil {
			utils.Error(fmt.Errorf("[Client:%s EventId:%s] failed to record terminal WhatsApp rejection in Redis: %v", data.Client, data.EventId, err))
			return false, false
		}
	}

	// Dual audit sinks via WriteOutput (MySQL WhatsappOutput + Marketing Output).
	// Production WriteOutput logs sink failures itself (SMS dual-write parity).
	if err := deps.WriteOutput(data, outputData); err != nil {
		return false, false
	}

	deleted, err := deps.Delete(data)
	if !deleted {
		utils.Error(fmt.Errorf("failed to delete terminal marketing WhatsApp: %v", err))
	}

	return true, deleted
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

	rcsResult, err := rcs.SendRcsByProcess(data)
	isMessageProcessed := rcsResult.Processed

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

	if ShouldSubmitZapCashMonitoring(data, rcsResult.Accepted) {
		if !monitoring.TrySubmit(monitoring.AcceptedResult{
			Payload:              data,
			ResolvedVendor:       rcsResult.ResolvedVendor,
			ResolvedTemplate:     rcsResult.ResolvedTemplate,
			TemplateVariables:    rcsResult.TemplateVariables,
			SMSFallbackVariables: rcsResult.SMSFallbackVariables,
			TransactionID:        rcsResult.TransactionID,
		}) {
			utils.Warn(fmt.Sprintf("[Client:%s CommId:%s] ZapCash monitoring copy dropped after accepted RCS send for vendor=%s template=%s",
				data.Client, data.CommId, rcsResult.ResolvedVendor, rcsResult.ResolvedTemplate))
		}
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
	marketing := isMarketingSMSDispatch(data)

	if marketing {
		skipSend, campaignDuplicate, redisTxn, redisErr, claimErr := claimOrSkipMarketingDispatch(data)
		if claimErr != nil {
			utils.Error(fmt.Errorf("[Client:%s EventId:%s] marketing SMS redis claim failed: %v", data.Client, data.EventId, claimErr))
			return false, false
		}

		if skipSend {
			// Blank Redis state means another worker is still in flight. A terminal
			// Redis state is a redelivery and must repair both audit sinks before ACK.
			if strings.TrimSpace(redisTxn) != "" || strings.TrimSpace(redisErr) != "" {
				replayResult := sms.TerminalReplayResult(data, redisTxn, redisErr)
				outputErr, trackingErr := RunParallelSMSPostSendWrites(
					func() error {
						return database.InsertData(config.Configs.SmsOutputTable, database.DBtechWrite, replayResult.DBData)
					},
					func() error {
						return recordMarketingSMSTrackingFromRedisSkip(data, redisTxn, redisErr)
					},
				)
				if outputErr != nil || trackingErr != nil {
					logSMSPostSendPersistenceFailure(data, outputErr, trackingErr)
					return false, false
				}
			}
			deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete duplicate in-flight marketing SMS: %v", delErr))
			}
			return true, deleted
		}

		if campaignDuplicate {
			if trackErr := recordMarketingSMSTracking(data, database.DispatchTrackingSkippedDuplicate,
				"", campaignDuplicateError(data)); trackErr != nil {
				return false, false
			}
			deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete campaign-duplicate marketing SMS: %v", delErr))
			}
			return true, deleted
		}

		data.CommId = sdkServices.ResolveCommID(data.CommId, data.Client)
	}

	if !AssignVendor(&data) {
		if marketing {
			if updErr := channelHelper.UpdateRedisErrorMessage(data, "requested vendor is not active"); updErr != nil {
				utils.Error(fmt.Errorf("[Client:%s EventId:%s] failed to record inactive vendor in redis: %v", data.Client, data.EventId, updErr))
				return false, false
			}
			if trackErr := recordMarketingSMSTracking(data, database.DispatchTrackingFailed, "", "requested vendor is not active"); trackErr != nil {
				return false, false
			}
		}
		// Terminal FAILED: keep Redis claims so a replay does not hit the vendor again
		// after tracking already exists.
		return rejectRequestedVendor(ctx, data, sqsClient, queueURL, msg)
	}

	if marketing {
		dbMappedData["CommId"] = data.CommId
		if err := database.InsertData(config.Configs.SdkSmsInputTable, database.DBtechWrite, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("[Client:%s CommId:%s] error inserting sms input audit: %v", data.Client, data.CommId, err))
		}
	}

	result, err := sms.SendSmsByProcessWithContext(ctx, data)

	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s CommId:%s] error in sending SMS: %v", data.Client, data.CommId, err))
		if marketing && result.AckSQS {
			if trackErr := recordMarketingSMSTrackingFromSend(data, result, err); trackErr != nil {
				return false, false
			}
		}

		if result.Processed && result.AckSQS {
			deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
			if !deleted {
				utils.Error(fmt.Errorf("failed to delete message after partial SMS processing: %v", delErr))
			}
		} else if marketing && !result.AckSQS {
			releaseMarketingDispatchClaims(data)
		}
		return result.Processed, deleted
	}

	if ShouldSubmitZapCashMonitoring(data, result.Accepted) {
		utils.Info(fmt.Sprintf("[Client:%s CommId:%s Channel:%s] submitting ZapCash monitoring copy for vendor=%s template=%s mobile_tail=%s",
			data.Client, data.CommId, data.Channel, result.ResolvedVendor, result.ResolvedTemplate, monitoring.MaskMobile(data.Mobile)))
		if !monitoring.TrySubmit(monitoring.AcceptedResult{
			Payload:           data,
			ResolvedVendor:    result.ResolvedVendor,
			ResolvedTemplate:  result.ResolvedTemplate,
			TemplateVariables: result.TemplateVariables,
			TransactionID:     result.TransactionID,
		}) {
			utils.Warn(fmt.Sprintf("[Client:%s CommId:%s] ZapCash monitoring copy dropped after accepted SMS send for vendor=%s template=%s",
				data.Client, data.CommId, result.ResolvedVendor, result.ResolvedTemplate))
		}
	}

	// Compliance failures are terminal only after both independent databases
	// confirm persistence. The independent idempotent writes run concurrently;
	// SQS acknowledgement still waits for both of them to complete successfully.
	if marketing && result.AckSQS && isWeCreditSMSComplianceFailure(result) {
		outputErr, trackingErr := RunParallelSMSPostSendWrites(
			func() error {
				return database.InsertData(config.Configs.SmsOutputTable, database.DBtechWrite, result.DBData)
			},
			func() error {
				return recordMarketingSMSTrackingFromSend(data, result, nil)
			},
		)

		if outputErr != nil || trackingErr != nil {
			logSMSPostSendPersistenceFailure(data, outputErr, trackingErr)
			releaseMarketingDispatchClaims(data)
			return false, false
		}

		if redisErr := channelHelper.UpdateRedisErrorMessage(data, complianceFailureMessage(result)); redisErr != nil {
			utils.Error(fmt.Errorf("[Client:%s SourceRowId:%d] failed to cache compliance result: %v", data.Client, data.SourceRowId, redisErr))
		}

		deleted, delErr = deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete compliance-blocked marketing SMS after persistence: %v", delErr))
		}

		return true, deleted
	}

	outputWritten := false
	if marketing && result.AckSQS {
		outputErr, trackingErr := RunParallelSMSPostSendWrites(
			func() error {
				return database.InsertData(config.Configs.SmsOutputTable, database.DBtechWrite, result.DBData)
			},
			func() error {
				return recordMarketingSMSTrackingFromSend(data, result, nil)
			},
		)
		outputWritten = true
		if outputErr != nil || trackingErr != nil {
			logSMSPostSendPersistenceFailure(data, outputErr, trackingErr)
			return false, false
		}
	} else if marketing && !result.AckSQS {
		releaseMarketingDispatchClaims(data)
	}

	if result.AckSQS {
		deleted, err = deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete message after SMS processing: %v", err))
		}
	} else {
		utils.Info(fmt.Sprintf("[Client:%s CommId:%s] retaining SQS message for non-terminal SMS outcome", data.Client, data.CommId))
	}

	if !outputWritten {
		if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtechWrite, result.DBData); err != nil {
			utils.Error(fmt.Errorf("error inserting data into sms output table for mobile %s: %v", data.Mobile, err))
		}
	}

	return result.Processed, deleted
}

func complianceFailureMessage(result sms.SendSmsResult) string {
	message, _ := result.DBData["ResponseMessage"].(string)
	return strings.TrimSpace(message)
}

// isWeCreditSMSComplianceFailure checks if the given result is a compliance failure for a WeCredit SMS
func isWeCreditSMSComplianceFailure(result sms.SendSmsResult) bool {
	message := complianceFailureMessage(result)
	return strings.Contains(message, "WECREDIT_SMS_CUTOFF") ||
		strings.Contains(message, "WECREDIT_SMS_EXPIRED") ||
		strings.Contains(message, "WECREDIT_SMS_CAMPAIGN_DATE_INVALID")
}

// RunParallelSMSPostSendWrites executes the independent audit writes together
// and waits for both results. Used by marketing SMS and WhatsApp. The caller
// must not acknowledge SQS unless both returned errors are nil.
func RunParallelSMSPostSendWrites(outputWrite, trackingWrite func() error) (outputErr, trackingErr error) {
	var group errgroup.Group
	group.Go(func() error {
		outputErr = outputWrite()
		return outputErr
	})
	group.Go(func() error {
		trackingErr = trackingWrite()
		return trackingErr
	})
	_ = group.Wait()
	return outputErr, trackingErr
}

func logSMSPostSendPersistenceFailure(data sdkModels.CommApiRequestBody, outputErr, trackingErr error) {
	outputStatus := "succeeded"
	if outputErr != nil {
		outputStatus = outputErr.Error()
	}

	trackingStatus := "succeeded"
	if trackingErr != nil {
		trackingStatus = trackingErr.Error()
	}

	utils.Error(fmt.Errorf(
		"[Client:%s CommId:%s EventId:%s] partial post-send persistence failure: sms_output=%s comm_dispatch_tracking=%s",
		data.Client, data.CommId, data.EventId, outputStatus, trackingStatus,
	))
}

func logWhatsappPostSendPersistenceFailure(data sdkModels.CommApiRequestBody, mysqlErr, marketingErr error) {
	mysqlStatus := "succeeded"
	if mysqlErr != nil {
		mysqlStatus = mysqlErr.Error()
	}
	marketingStatus := "succeeded"
	if marketingErr != nil {
		marketingStatus = marketingErr.Error()
	}

	utils.Error(fmt.Errorf(
		"[Client:%s CommId:%s EventId:%s] partial post-send persistence failure: whatsapp_output=%s comm_whatsapp_marketing_output=%s",
		data.Client, data.CommId, data.EventId, mysqlStatus, marketingStatus,
	))
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
	} else if data.Channel == variables.PUSH {
		// Channel-scoped empty-vendor default (Email→SINCH pattern). No IsVendorActive
		// on this hardcode; ShouldHitVendor in push.Send is the kill switch.
		data.Vendor = variables.FCM
	} else {
		data.Vendor = GetVendorByClientAndChannel(data.Channel, data.Client, data.CommId)
		utils.Debug(fmt.Sprintf("Assigned vendor: %s for client: %s, channel: %s, commId: %s", data.Vendor, data.Client, data.Channel, data.CommId))
	}
	return true
}

func rejectRequestedVendor(ctx context.Context, data sdkModels.CommApiRequestBody, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	if data.IsMonitorCopy {
		utils.Warn(fmt.Sprintf("ZapCash monitoring copy rejected because pinned vendor is inactive channel=%s stage=%.2f vendor=%s commId=%s",
			data.Channel, data.Stage, data.Vendor, data.CommId))
	}

	deleted, err := deleteMessage(ctx, sqsClient, queueURL, msg, data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to delete message rejected for inactive requested vendor: %v", err))
	}
	return true, deleted
}

// ShouldSubmitZapCashMonitoring is the pure production-handler gate. TrySubmit repeats
// the identity checks defensively, but no ineligible result should reach that boundary.
func ShouldSubmitZapCashMonitoring(data sdkModels.CommApiRequestBody, providerAccepted bool) bool {
	if !providerAccepted || data.IsMonitorCopy || !strings.EqualFold(strings.TrimSpace(data.Client), "zapcash") {
		return false
	}

	switch strings.ToUpper(strings.TrimSpace(data.Channel)) {
	case variables.SMS, variables.RCS, variables.WhatsApp:
		return true
	default:
		return false
	}
}

func isMarketingSMSDispatch(data sdkModels.CommApiRequestBody) bool {
	return strings.EqualFold(strings.TrimSpace(data.Source), "marketing") && data.SourceRowId != 0
}

func isMarketingWPDispatch(data sdkModels.CommApiRequestBody) bool {
	return strings.EqualFold(strings.TrimSpace(data.Source), "marketing") && data.SourceRowId != 0
}

func claimOrSkipMarketingDispatch(data sdkModels.CommApiRequestBody) (skipSend, campaignDuplicate bool, redisTxn, redisErr string, err error) {
	redisKey := channelHelper.GenerateRedisKeyForRequest(data)
	exists, txn, errMsg, err := redis.GetMobileDataFromRedis(config.Configs.CommIdempotentKey, redisKey, redis.RDB)
	if err != nil {
		return false, false, "", "", err
	}

	if exists {
		errLower := strings.ToLower(strings.TrimSpace(errMsg))
		if strings.TrimSpace(txn) == "" && strings.Contains(errLower, "shouldhitvendor is off") && channelHelper.ShouldHitVendor(data.Client, data.Channel) {
			if relErr := redis.ReleaseMobileChannelHashField(redis.RDB, config.Configs.CommIdempotentKey, redisKey); relErr != nil {
				return false, false, "", "", relErr
			}

			utils.Info(fmt.Sprintf("Reclaimed stale shouldHitVendor redis block for EventId=%s (previous: %s)", data.EventId, errMsg))
			exists = false
		} else {
			return true, false, txn, errMsg, nil
		}
	}

	if !exists {
		if setErr := redis.SetMobileChannelKey(redis.RDB, config.Configs.CommIdempotentKey, redisKey); setErr != nil {
			if strings.Contains(setErr.Error(), "already exists") {
				_, txn, errMsg, getErr := redis.GetMobileDataFromRedis(config.Configs.CommIdempotentKey, redisKey, redis.RDB)
				if getErr != nil {
					return false, false, "", "", getErr
				}
				return true, false, txn, errMsg, nil
			}
			return false, false, "", "", setErr
		}
	}
	if !channelHelper.IsMarketingCampaignRequest(data) {
		return false, false, "", "", nil
	}

	eventID := strings.TrimSpace(data.EventId)
	campaignKey := channelHelper.GenerateMarketingCampaignDedupKey(data)
	claimed, claimErr := redis.ClaimMarketingCampaignDedupKey(redis.RDB, campaignKey, eventID)
	if claimErr != nil {
		_ = redis.ReleaseMobileChannelHashField(redis.RDB, config.Configs.CommIdempotentKey, redisKey)
		return false, false, "", "", claimErr
	}
	if claimed {
		return false, false, "", "", nil
	}
	if redis.RDB != nil {
		existing, getErr := redis.RDB.Get(context.Background(), campaignKey).Result()
		if getErr == nil && strings.TrimSpace(existing) == eventID {
			return false, false, "", "", nil
		}
	}
	return false, true, "", "", nil
}

func releaseMarketingDispatchClaims(data sdkModels.CommApiRequestBody) {
	redisKey := channelHelper.GenerateRedisKeyForRequest(data)
	_, _ = redis.ReclaimBlankMobileChannelKey(redis.RDB, config.Configs.CommIdempotentKey, redisKey)
	if channelHelper.IsMarketingCampaignRequest(data) {
		_ = redis.ReleaseMarketingCampaignDedupKey(redis.RDB, channelHelper.GenerateMarketingCampaignDedupKey(data))
	}
}

func campaignDuplicateError(data sdkModels.CommApiRequestBody) string {
	client := strings.ToLower(strings.TrimSpace(data.Client))
	channel := strings.ToUpper(strings.TrimSpace(data.Channel))
	if client == "wecredit" && channel == "WHATSAPP" {
		return fmt.Sprintf("whatsapp already sent today for mobile %s", strings.TrimSpace(data.Mobile))
	}

	return fmt.Sprintf("campaign duplicate: channel %s process %s event_id %s already sent today",
		channel,
		strings.ToLower(strings.TrimSpace(data.ProcessName)),
		strings.TrimSpace(data.EventId))
}

// recordBlankMarketingWhatsappClaim records a blank marketing WhatsApp claim.
func recordBlankMarketingWhatsappClaim(data sdkModels.CommApiRequestBody, msg *sqs.Message, redriveMaxReceiveCount int) {
	receiveCount := approximateReceiveCount(msg)
	fields := map[string]interface{}{
		"reason":                  "blank_redis_claim",
		"lane":                    "wecredit-whatsapp",
		"client":                  strings.ToLower(strings.TrimSpace(data.Client)),
		"eventId":                 strings.TrimSpace(data.EventId),
		"sourceRowId":             data.SourceRowId,
		"sqsMessageId":            aws.StringValue(messageID(msg)),
		"approximateReceiveCount": receiveCount,
	}

	raw, err := json.Marshal(fields)
	if err == nil {
		utils.Error(errors.New(string(raw)))
	}

	metrics.CountByReason("MarketingWhatsappBlankRedisClaim", "wecredit-whatsapp", "blank_redis_claim", 1)
	metrics.CountByReason("MarketingWhatsappRetry", "wecredit-whatsapp", "blank_redis_claim", 1)

	// Check if the message should be emitted to the DLQ.
	if ShouldEmitWhatsappDLQImminent(msg, redriveMaxReceiveCount) {
		metrics.CountByReason("MarketingWhatsappDLQImminent", "wecredit-whatsapp", "blank_redis_claim", 1)
	}
}

func ShouldEmitWhatsappDLQImminent(msg *sqs.Message, redriveMaxReceiveCount int) bool {
	return redriveMaxReceiveCount > 0 && approximateReceiveCount(msg) >= redriveMaxReceiveCount
}

func messageID(msg *sqs.Message) *string {
	if msg == nil {
		return nil
	}
	return msg.MessageId
}

func approximateReceiveCount(msg *sqs.Message) int {
	if msg == nil || msg.Attributes == nil {
		return 1
	}
	count, err := strconv.Atoi(strings.TrimSpace(aws.StringValue(msg.Attributes["ApproximateReceiveCount"])))
	if err != nil || count < 1 {
		return 1
	}
	return count
}

func mapString(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

func mapBool(data map[string]interface{}, key string) bool {
	if data == nil {
		return false
	}
	switch value := data[key].(type) {
	case bool:
		return value
	case int:
		return value == 1
	case int64:
		return value == 1
	case float64:
		return value == 1
	default:
		return false
	}
}

// MapMarketingWhatsappOutput projects SDK send DBData onto CommWhatsappMarketingOutput columns.
func MapMarketingWhatsappOutput(data sdkModels.CommApiRequestBody, output map[string]interface{}) map[string]interface{} {
	row := map[string]interface{}{
		"SourceRowId":    data.SourceRowId,
		"EventId":        data.EventId,
		"CommId":         data.CommId,
		"Mobile":         data.Mobile,
		"Vendor":         data.Vendor,
		"Process":        data.ProcessName,
		"Client":         data.Client,
		"TemplateName":   data.TemplateReference,
		"AppId":          data.AppId,
		"Tag1":           data.Tag1,
		"Tag2":           data.Tag2,
		"DynamicMobile":  data.DynamicMobile,
		"VariablesValue": data.TemplateVariableValues,
		"HitTime":        time.Now(),
	}

	if !data.ScheduledAt.IsZero() {
		row["ScheduledAt"] = data.ScheduledAt
	}

	if output != nil {
		if v, ok := output["AppId"]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				row["AppId"] = strings.TrimSpace(s)
			}
		}

		if v, ok := output["TransactionId"]; ok {
			row["TransactionId"] = v
			row["MessageId"] = v
		}

		if v, ok := output["ResponseMessage"]; ok {
			row["ResponseMessage"] = v
		}

		if v, ok := output["IsSent"]; ok {
			row["IsSent"] = mapBool(output, "IsSent") || v == 1 || v == true || v == "1"
		}

		if v, ok := output["RawPayload"]; ok {
			row["RawPayload"] = v
		}

		if v, ok := output["RawResponse"]; ok {
			row["RawResponse"] = v
		}

		if v, ok := output["MobileNumber"]; ok {
			if mobile, _ := row["Mobile"].(string); strings.TrimSpace(mobile) == "" {
				row["Mobile"] = v
			}
		}
	}

	return row
}

// MapMarketingWhatsappMysqlOutput projects terminal WA outcomes onto MySQL WhatsappOutputTable
// columns (lender-shaped audit). Used alongside CommWhatsappMarketingOutput — SMS parity
// with SmsOutputTable + tracking.
func MapMarketingWhatsappMysqlOutput(data sdkModels.CommApiRequestBody, output map[string]interface{}) map[string]interface{} {
	row := map[string]interface{}{
		"CommId":       data.CommId,
		"Vendor":       data.Vendor,
		"MobileNumber": data.Mobile,
		"IsSent":       false,
	}

	if name := strings.TrimSpace(data.TemplateReference); name != "" {
		row["TemplateName"] = name
	}
	if appID := strings.TrimSpace(data.AppId); appID != "" {
		row["AppId"] = appID
	}

	if output == nil {
		return row
	}

	if v, ok := output["CommId"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			row["CommId"] = strings.TrimSpace(s)
		}
	}

	if v, ok := output["Vendor"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			row["Vendor"] = strings.TrimSpace(s)
		}
	}

	if v, ok := output["MobileNumber"]; ok {
		row["MobileNumber"] = v
	} else if v, ok := output["Mobile"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			row["MobileNumber"] = strings.TrimSpace(s)
		}
	}

	if v, ok := output["TransactionId"]; ok {
		row["TransactionId"] = v
	}

	if v, ok := output["ResponseMessage"]; ok {
		row["ResponseMessage"] = v
	}

	if v, ok := output["TemplateName"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			row["TemplateName"] = strings.TrimSpace(s)
		}
	}

	if v, ok := output["AppId"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			row["AppId"] = strings.TrimSpace(s)
		}
	}

	if v, ok := output["PaymentLink"]; ok {
		row["PaymentLink"] = v
	}

	if v, ok := output["RawPayload"]; ok {
		row["RawPayload"] = v
	}

	if v, ok := output["RawResponse"]; ok {
		row["RawResponse"] = v
	}

	if _, ok := output["IsSent"]; ok {
		row["IsSent"] = mapBool(output, "IsSent") || output["IsSent"] == 1 || output["IsSent"] == true || output["IsSent"] == "1"
	}

	return row
}

// recordMarketingSMSTrackingFromRedisSkip writes tracking when Redis already has a
// terminal result from an earlier send (no vendor call). Blank Redis is in-flight:
// the first copy still owns the tracking insert.
func recordMarketingSMSTrackingFromRedisSkip(data sdkModels.CommApiRequestBody, redisTxn, redisErr string) error {
	txn := strings.TrimSpace(redisTxn)
	errMsg := strings.TrimSpace(redisErr)
	if txn == "" && errMsg == "" {
		return nil
	}
	outcome := database.DispatchTrackingSent
	if txn == "" {
		outcome = database.DispatchTrackingFailed
	} else {
		errMsg = ""
	}
	return recordMarketingSMSTracking(data, outcome, txn, errMsg)
}

func recordMarketingSMSTrackingFromSend(data sdkModels.CommApiRequestBody, result sms.SendSmsResult, sendErr error) error {
	txn, _ := result.DBData["TransactionId"].(string)
	errMsg, _ := result.DBData["ResponseMessage"].(string)
	if sendErr != nil && errMsg == "" {
		errMsg = sendErr.Error()
	}
	status := database.DispatchTrackingFailed
	if result.DBData["IsSent"] == 1 {
		status = database.DispatchTrackingSent
		errMsg = ""
	}
	return recordMarketingSMSTracking(data, status, txn, errMsg)
}

func recordMarketingSMSTracking(data sdkModels.CommApiRequestBody, outcome, transactionId, errorMessage string) error {
	err := database.InsertCommDispatchTracking(database.DBMarketing, config.Configs.CommMarketingInputTable, config.Configs.CommDispatchTrackingTable, database.CommDispatchTrackingRow{
		Source:            data.Source,
		SourceRowId:       data.SourceRowId,
		Channel:           "SMS",
		Client:            data.Client,
		Vendor:            data.Vendor,
		Process:           data.ProcessName,
		EventId:           data.EventId,
		CommId:            data.CommId,
		TransactionId:     transactionId,
		Outcome:           outcome,
		ErrorMessage:      errorMessage,
		TemplateReference: data.TemplateReference,
	})
	if errors.Is(err, database.ErrDispatchTrackingAlreadyExists) {
		utils.Info(fmt.Sprintf("dispatch tracking already recorded for sourceRowId=%d", data.SourceRowId))
		return nil
	}

	if errors.Is(err, database.ErrDispatchSourceAlreadyTerminal) {
		utils.Info(fmt.Sprintf("dispatch source already terminal sourceRowId=%d; preserving first terminal outcome", data.SourceRowId))
		return nil
	}

	if err != nil {
		utils.Error(fmt.Errorf("failed to insert dispatch tracking sourceRowId=%d: %v", data.SourceRowId, err))
		return err
	}
	return nil
}
