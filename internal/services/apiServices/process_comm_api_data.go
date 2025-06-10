package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/google/uuid"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
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

func ProcessCommApiData(data *sdkModels.CommApiRequestBody, snsClient *sns.SNS) (sdkModels.CommApiResponseBody, error) {
	isValidate, message := helper.ValidateCommRequest(*data)

	if !isValidate {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("%s", message)
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
	err = queue.SendMessageToAwsQueue(snsClient, dataMap, config.SdkConfigs.AwsSnsArn, subject)
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
