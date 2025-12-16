package sdkServices

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/google/uuid"
	channelHelper "github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/redis"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// GenerateCommID generates a unique lead ID using the UUID library
func GenerateCommID() string {
	// Generate a new UUID
	newUUID, err := uuid.NewUUID()
	if err != nil {
		utils.Error(fmt.Errorf("failed to generate comm ID: %v", err))
	}
	// Get current timestamp in nanoseconds
	timestamp := time.Now().UnixNano()
	// Combine UUID and timestamp to ensure uniqueness
	commID := fmt.Sprintf("WC-%s-%d", newUUID.String(), timestamp)
	return commID
}

func ProcessCommApiData(data *sdkModels.CommApiRequestBody, snsClient *sns.SNS, topicArn, redisAddress, redisHashKey string) (sdkModels.CommApiResponseBody, error) {
	isValidate, message := sdkHelper.ValidateCommRequest(*data)

	if !isValidate {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("%s", message)
	}

	redisClient, err := redis.GetSdkRedisClient(redisAddress)
	if err != nil {
		utils.Error(fmt.Errorf("failed to get redis client for address: %s: %v", redisAddress, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("internal server error")
	}

	redisKey := channelHelper.GenerateRedisKey(data.Mobile, data.Channel, data.Stage)

	exists, transactionId, _, err := redis.GetMobileDataFromRedis(redisHashKey, redisKey, redisClient)
	if err != nil {
		utils.Error(fmt.Errorf("failed to get mobile data from redis for mobile: %s, channel: %s and stage: %.0f: %v", data.Mobile, data.Channel, data.Stage, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("internal server error")
	}
	if exists {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("message already processed for mobile: %s, channel: %s and stage: %.0f. TransactionId: %s", data.Mobile, data.Channel, data.Stage, transactionId)
	} else {
		err = redis.SetMobileChannelKey(redisClient, redisHashKey, redisKey)
		if err != nil {
			utils.Error(fmt.Errorf("failed to set mobile channel key in redis for mobile: %s, channel: %s and stage: %.0f: %v", data.Mobile, data.Channel, data.Stage, err))
			return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to set mobile channel key in redis for mobile: %s, channel: %s and stage: %.0f: %v", data.Mobile, data.Channel, data.Stage, err)
		}
	}

	// Set CommId for requested Data
	data.CommId = GenerateCommID()

	subject := variables.NonPriority

	if data.IsPriority {
		subject = variables.Priority
	}

	// Convert the struct to JSON (byte slice)
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to serialize data: %w", err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to serialize data: %w", err)
	}

	// Initialize the map
	var dataMap map[string]interface{}

	// Deserialize JSON into map
	err = json.Unmarshal(jsonBytes, &dataMap)
	if err != nil {
		utils.Error(fmt.Errorf("failed to convert data to map: %w", err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to convert data to map: %w", err)
	}

	// Send the map to AWS Queue
	err = queue.SendMessageToAwsQueue(snsClient, dataMap, topicArn, subject)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while sending data to queue: %w", err))
		return sdkModels.CommApiResponseBody{
			Success: false,
		}, fmt.Errorf("error occurred while sending data to queue: %w", err)
	}

	utils.Info("Message sent to AWS SNS.")

	return sdkModels.CommApiResponseBody{
		Success: true,
		CommId:  data.CommId,
	}, nil
}
