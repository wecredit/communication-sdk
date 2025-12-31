package sdkServices

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/google/uuid"
	"github.com/wecredit/communication-sdk/internal/database"
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

func ProcessCommApiData(data *sdkModels.CommApiRequestBody, snsClient *sns.SNS, topicArn string) (sdkModels.CommApiResponseBody, error) {
	isValidate, message := sdkHelper.ValidateCommRequest(*data)

	if !isValidate {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("%s", message)
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
