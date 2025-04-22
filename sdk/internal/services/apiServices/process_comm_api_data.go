package services

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/queue"
	"github.com/wecredit/communication-sdk/sdk/internal/utils"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
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

func ProcessCommApiData(data sdkModels.CommApiRequestBody) (int, sdkModels.CommApiResponseBody) {

	/* 	isValidate, message := helper.ValidateCommRequest(data)

	   	if !isValidate {
	   		return http.StatusBadRequest, models.CommApiResponseBody{
	   			StatusCode:    status.LeadDataValidationFailed,
	   			StatusMessage: message,
	   		}
	   	}
	*/
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
	fmt.Println("Azure :======", config.Configs.AzureTopicName)
	err = queue.SendMessage(dataMap, config.Configs.AzureTopicName)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while sending data to queue: %w", err))
		return http.StatusInternalServerError, sdkModels.CommApiResponseBody{
			Success: false,
		}
	}
	utils.Info("Message sent to Azure Queue.")

	return http.StatusOK, sdkModels.CommApiResponseBody{
		Success: true,
		CommId:  CommId,
	}
}
