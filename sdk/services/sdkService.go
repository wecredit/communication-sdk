package sdkServices

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/database"
	redisInteraction "github.com/wecredit/communication-sdk/internal/redis"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	dbservices "github.com/wecredit/communication-sdk/internal/services/dbService"
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

func ProcessCommApiData(data *sdkModels.CommApiRequestBody, snsClient *sns.SNS, topicArn string, redisClient *redis.Client) (sdkModels.CommApiResponseBody, error) {
	isValidate, message := sdkHelper.ValidateCommRequest(*data)

	if !isValidate {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("%s", message)
	}

	// Convert stage to string for Redis key
	redisKey := channelHelper.GenerateRedisKey(data.Mobile, data.Channel, data.Stage)

	// check if message already sent for once
	exists, transactionId, errorMessage, err := redisInteraction.GetMobileDataFromRedis(config.Configs.CommIdempotentKey, redisKey, redisClient)
	if err != nil {
		utils.Error(fmt.Errorf("error in checking mobile: %s, redisKey: %s on redis: %v", data.Mobile, redisKey, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error in checking mobile: %s, redisKey: %s on redis: %v", data.Mobile, redisKey, err)
	}

	// If we have data from Redis, handle accordingly
	if exists {
		// Priority: If we have a transactionId, the message was successfully processed before
		if transactionId != "" {
			// check if record already exists in output table
			dataExistsAlready, err := services.CheckIfDataAlreadyExists(*data, redisKey, transactionId)
			if err != nil {
				utils.Error(fmt.Errorf("error checking if data exists for mobile: %s, redisKey: %s, transactionId: %s: %v", data.Mobile, redisKey, transactionId, err))
				return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error checking if data exists for mobile: %s, redisKey: %s, transactionId: %s: %v", data.Mobile, redisKey, transactionId, err)
			}

			// for debugging purpose
			if dataExistsAlready {
				utils.Debug(fmt.Sprintf("Data already exists in output table for mobile: %s and channel: %s, redisKey: %s, transactionId: %s", data.Mobile, data.Channel, redisKey, transactionId))
				return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("data already exists in output table for mobile: %s and channel: %s, redisKey: %s, transactionId: %s", data.Mobile, data.Channel, redisKey, transactionId)
			}

			utils.Debug(fmt.Sprintf("Data does not exist in output table for mobile: %s and channel: %s, redisKey: %s, transactionId: %s", data.Mobile, data.Channel, redisKey, transactionId))
			return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("data does not exist in output table for mobile: %s and channel: %s, redisKey: %s, transactionId: %s", data.Mobile, data.Channel, redisKey, transactionId)
		}

		// If we have an error message (and no transactionId), return error
		if errorMessage != "" && transactionId == "" {
			return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("message already processed for mobile: %s and channel: %s, redisKey: %s with error: %s", data.Mobile, data.Channel, redisKey, errorMessage)
		}

		// Redis key exists but no transactionId or errorMessage - return error
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("message already processed for redisKey: %s (key exists but no transactionId/errorMessage)", redisKey)
	}

	// If not exists, add key with blank value
	err = redisInteraction.SetMobileChannelKey(redisClient, config.Configs.CommIdempotentKey, redisKey)
	if err != nil {
		utils.Error(fmt.Errorf("redis add failed for mobile: %s, redisKey: %s: %v", data.Mobile, redisKey, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("redis add failed for mobile: %s and channel: %s, redisKey: %s: %v", data.Mobile, data.Channel, redisKey, err)
	}

	// Set CommId for requested Data
	data.CommId = GenerateCommID()

	subject := variables.NonPriority

	if data.IsPriority {
		subject = variables.Priority
	}

	dbMappedData, err := dbservices.MapIntoDbModel(data)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel for mobile %s and channel %s: %v", data.Mobile, data.Channel, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error in mapping data into dbModel for mobile %s and channel %s: %v", data.Mobile, data.Channel, err)
	}

	if data.Channel == variables.Email {
		delete(dbMappedData, "Mobile")
		dbMappedData["Email"] = data.Email
	}

	// Convert the struct to JSON (byte slice)
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to serialize data for mobile %s and channel %s: %w", data.Mobile, data.Channel, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to serialize data for mobile %s and channel %s: %w", data.Mobile, data.Channel, err)
	}

	// Initialize the map
	var dataMap map[string]interface{}

	// Deserialize JSON into map
	err = json.Unmarshal(jsonBytes, &dataMap)
	if err != nil {
		utils.Error(fmt.Errorf("failed to convert data to map for mobile %s and channel %s: %w", data.Mobile, data.Channel, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to convert data to map for mobile %s and channel %s: %w", data.Mobile, data.Channel, err)
	}

	if err := database.InsertData(data.InputTableName, data.DbClient, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into input table %s for mobile %s and channel %s: %v", data.InputTableName, data.Mobile, data.Channel, err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error inserting data into input table %s for mobile %s and channel %s: %v", data.InputTableName, data.Mobile, data.Channel, err)
	}
	// Send the map to AWS Queue
	err = queue.SendMessageToAwsQueue(snsClient, dataMap, topicArn, subject)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while sending data to queue for mobile %s and channel %s: %w", data.Mobile, data.Channel, err))
		return sdkModels.CommApiResponseBody{
			Success: false,
		}, fmt.Errorf("error occurred while sending data to queue for mobile %s and channel %s: %w", data.Mobile, data.Channel, err)
	}

	utils.Info(fmt.Sprintf("Message sent to AWS SNS for mobile %s and channel %s for stage %f", data.Mobile, data.Channel, data.Stage))

	return sdkModels.CommApiResponseBody{Success: true, CommId: data.CommId}, nil
}
