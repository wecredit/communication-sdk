package services

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// GenerateCommID generates a unique lead ID using the UUID library
func GenerateCommID() string {
	// Generate a new UUID
	newUUID, err := uuid.NewUUID()
	if err != nil {
		utils.Error(fmt.Errorf("failed to generate lead ID: %w", err))
		return ""
	}

	// Format the UUID as a string and return it
	return newUUID.String()
}

func ProcessCommApiData(data sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	isValidate, message := helper.ValidateCommRequest(data)

	if !isValidate {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("%s", message)
	}
	// Set CommId for requested Data
	CommId := GenerateCommID()

	// Convert the struct to JSON (byte slice)
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		utils.Error(fmt.Errorf("failed to serialize data: %w", err))
	}

	// Initialize the map
	var dataMap map[string]interface{}

	// Deserialize JSON into map
	err = json.Unmarshal(jsonBytes, &dataMap)
	if err != nil {
		utils.Error(fmt.Errorf("failed to convert data to map: %w", err))
	}

	// Define topic subject based on source
	// var subject string

	// if data.Source == variables.MasterApi || data.Source == variables.MasterApiTest {
	// 	subject = variables.MasterData
	// } else {
	// 	subject = variables.NonMasterData
	// }

	// Send the map to Azure Queue
	fmt.Println("Azure :======")
	fmt.Println("config:", config.Configs.QueueTopicName)
	fmt.Println("datamap:", dataMap)

	err = queue.SendMessage(dataMap, config.Configs.QueueTopicName)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while sending data to queue: %w", err))
		return sdkModels.CommApiResponseBody{
			Success: false,
		}, fmt.Errorf("error occurred while sending data to queue: %w", err)
	}
	utils.Info("Message sent to Azure Queue.")

	return sdkModels.CommApiResponseBody{
		Success: true,
		CommId:  CommId,
	}, nil
}
