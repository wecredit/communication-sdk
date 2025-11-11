package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/config"
	email "github.com/wecredit/communication-sdk/internal/channels/email"
	rcs "github.com/wecredit/communication-sdk/internal/channels/rcs"
	sms "github.com/wecredit/communication-sdk/internal/channels/sms"
	"github.com/wecredit/communication-sdk/internal/channels/whatsapp"
	"github.com/wecredit/communication-sdk/internal/database"
	dbservices "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// ErrorQueueMessage represents the message structure in error queue
type ErrorQueueMessage struct {
	Payload      interface{} `json:"payload"`
	OriginalData sdkModels.CommApiRequestBody
	TableName    string // For OutputInsertionFails
	ErrorSubject string
	ErrorMessage string
}

// ProcessErrorQueueMessage processes a single message from error queue
func ProcessErrorQueueMessage(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) bool {
	// Extract message attributes
	subject := ""

	if msg.MessageAttributes != nil {
		if attr, ok := msg.MessageAttributes["Subject"]; ok && attr.StringValue != nil {
			subject = *attr.StringValue
		}
	}

	// Parse the message body
	var data sdkModels.CommApiRequestBody
	if err := parseMessageBody(msg.Body, &data); err != nil {
		utils.Error(fmt.Errorf("failed to parse error queue message body: %v", err))
		return false
	}

	utils.Info(fmt.Sprintf("[ErrorQueue] Processing message with subject: %s for CommId: %s", subject, data.CommId))

	// Route based on error subject
	switch subject {
	case variables.InputInsertionFails:
		return handleInputInsertionRetry(ctx, sqsClient, queueURL, msg, data)
	case variables.ApiHitsFails:
		return handleApiHitRetry(ctx, sqsClient, queueURL, msg, data)
	case variables.OutputInsertionFails:
		return handleOutputInsertionRetry(ctx, sqsClient, queueURL, msg, data, string(*msg.Body))
	default:
		utils.Error(fmt.Errorf("[ErrorQueue] unknown error subject: %s for CommId: %s", subject, data.CommId))
		return false
	}
}

// parseMessageBody parses the message body, handling both direct CommApiRequestBody and map[string]interface{}
func parseMessageBody(body *string, data *sdkModels.CommApiRequestBody) error {
	if body == nil {
		return fmt.Errorf("message body is nil")
	}

	// Try to unmarshal as CommApiRequestBody first
	if err := json.Unmarshal([]byte(*body), data); err == nil {
		return nil
	}

	// If that fails, try to unmarshal as map and extract CommApiRequestBody
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(*body), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Try to find the actual data
	if commId, ok := payload["CommId"].(string); ok {
		data.CommId = commId
		data.Mobile = getStringValue(payload, "Mobile", "MobileNumber")
		data.Email = getStringValue(payload, "Email")
		data.Channel = getStringValue(payload, "Channel")
		data.ProcessName = getStringValue(payload, "ProcessName")
		data.Client = getStringValue(payload, "Client")
		data.Vendor = getStringValue(payload, "Vendor")
		data.Stage = getFloatValue(payload, "Stage")
		data.IsPriority = getBoolValue(payload, "IsPriority")
		data.EmiAmount = getStringValue(payload, "EmiAmount")
		data.CustomerName = getStringValue(payload, "CustomerName")
		data.LoanId = getStringValue(payload, "LoanId")
		data.ApplicationNumber = getStringValue(payload, "ApplicationNumber")
		data.DueDate = getStringValue(payload, "DueDate")
		data.Description = getStringValue(payload, "Description")
		data.PaymentLink = getStringValue(payload, "PaymentLink")
		data.AzureIdempotencyKey = getStringValue(payload, "AzureIdempotencyKey")
		return nil
	}

	return fmt.Errorf("could not parse message body into CommApiRequestBody")
}

// Helper functions for parsing map[string]interface{}
func getStringValue(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := m[key].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

func getFloatValue(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0
}

func getBoolValue(m map[string]interface{}, key string) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return false
}

// handleInputInsertionRetry handles retry for input insertion failures (Step 1)
func handleInputInsertionRetry(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, data sdkModels.CommApiRequestBody) bool {
	utils.Info(fmt.Sprintf("[ErrorQueue] Retrying input insertion for CommId: %s", data.CommId))

	// Normalize data
	data.Client = strings.ToLower(data.Client)
	data.Channel = strings.ToUpper(data.Channel)
	data.ProcessName = strings.ToUpper(data.ProcessName)

	if data.AzureIdempotencyKey == "" {
		data.AzureIdempotencyKey = fmt.Sprintf("%s_%s", strings.ToLower(data.ProcessName), strings.ToLower(data.Description))
	}

	// Map to DB model
	dbMappedData, err := dbservices.MapIntoDbModel(data)
	if err != nil {
		utils.Error(fmt.Errorf("[ErrorQueue] error mapping data to dbModel for CommId: %s: %v", data.CommId, err))
		return false
	}

	// Execute input insertion, API hit, and output insertion
	success := executeCompleteFlow(ctx, sqsClient, queueURL, msg, data, dbMappedData, true, true, true)

	if success {
		deleteMessage(ctx, sqsClient, queueURL, msg, data)
	}

	return success
}

// handleApiHitRetry handles retry for API hit failures (Step 2)
func handleApiHitRetry(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, data sdkModels.CommApiRequestBody) bool {
	utils.Info(fmt.Sprintf("[ErrorQueue] Retrying API hit for CommId: %s", data.CommId))

	// Normalize data
	data.Client = strings.ToLower(data.Client)
	data.Channel = strings.ToUpper(data.Channel)
	data.ProcessName = strings.ToUpper(data.ProcessName)

	if data.AzureIdempotencyKey == "" {
		data.AzureIdempotencyKey = fmt.Sprintf("%s_%s", strings.ToLower(data.ProcessName), strings.ToLower(data.Description))
	}

	// Get vendor assignment
	AssignVendor(&data)

	// Execute API hit and output insertion only (input insertion already done)
	success := executeCompleteFlow(ctx, sqsClient, queueURL, msg, data, nil, false, true, true)

	if success {
		deleteMessage(ctx, sqsClient, queueURL, msg, data)
	}

	return success
}

// handleOutputInsertionRetry handles retry for output insertion failures (Step 3)
func handleOutputInsertionRetry(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, data sdkModels.CommApiRequestBody, rawBody string) bool {
	utils.Info(fmt.Sprintf("[ErrorQueue] Retrying output insertion for CommId: %s", data.CommId))

	// Parse the dbMappedData from raw body
	var dbMappedData map[string]interface{}
	if err := json.Unmarshal([]byte(rawBody), &dbMappedData); err != nil {
		utils.Error(fmt.Errorf("[ErrorQueue] failed to parse dbMappedData for CommId: %s: %v", data.CommId, err))
		return false
	}

	// Determine the output table name from channel
	var tableName string
	switch data.Channel {
	case variables.WhatsApp:
		tableName = config.Configs.WhatsappOutputTable
	case variables.SMS:
		tableName = config.Configs.SmsOutputTable
	case variables.Email:
		tableName = config.Configs.EmailOutputTable
		delete(dbMappedData, "MobileNumber")
		dbMappedData["Email"] = data.Email
	case variables.RCS:
		tableName = config.Configs.RcsOutputTable
	default:
		utils.Error(fmt.Errorf("[ErrorQueue] invalid channel: %s for CommId: %s", data.Channel, data.CommId))
		return false
	}

	// Execute output insertion only
	if err := database.InsertData(tableName, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("[ErrorQueue] output insertion retry failed for CommId: %s: %v", data.CommId, err))
		// Note: We don't send it back to error queue to avoid infinite loops
		return false
	}

	utils.Info(fmt.Sprintf("[ErrorQueue] Output insertion retry successful for CommId: %s", data.CommId))
	deleteMessage(ctx, sqsClient, queueURL, msg, data)
	return true
}

// executeCompleteFlow executes the complete processing flow with configurable steps
func executeCompleteFlow(ctx context.Context, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, doInputInsertion, doApiHit, doOutputInsertion bool) bool {

	// Step 1: Input Insertion (if needed)
	if doInputInsertion {
		var inputTable string
		switch data.Channel {
		case variables.WhatsApp:
			inputTable = config.Configs.SdkWhatsappInputTable
		case variables.SMS:
			inputTable = config.Configs.SdkSmsInputTable
		case variables.Email:
			inputTable = config.Configs.SdkEmailInputTable
			delete(dbMappedData, "Mobile")
			dbMappedData["Email"] = data.Email
		case variables.RCS:
			inputTable = config.Configs.SdkRcsInputTable
		default:
			utils.Error(fmt.Errorf("[ErrorQueue] invalid channel: %s for CommId: %s", data.Channel, data.CommId))
			return false
		}

		if err := database.InsertData(inputTable, database.DBtechWrite, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("[ErrorQueue] input insertion failed for CommId: %s: %v", data.CommId, err))
			return false
		}
		utils.Info(fmt.Sprintf("[ErrorQueue] Input insertion successful for CommId: %s", data.CommId))
	}

	// Step 2: API Hit (if needed)
	if doApiHit {
		var err error

		switch data.Channel {
		case variables.WhatsApp:
			_, dbMappedData, err = whatsapp.SendWpByProcess(data)
		case variables.SMS:
			_, dbMappedData, err = sms.SendSmsByProcess(data)
		case variables.Email:
			_, dbMappedData, err = email.SendEmailByProcess(data)
		case variables.RCS:
			_, err = rcs.SendRcsByProcess(data)
			// For RCS, dbMappedData is handled inside the function
		default:
			utils.Error(fmt.Errorf("[ErrorQueue] invalid channel: %s for CommId: %s", data.Channel, data.CommId))
			return false
		}

		if err != nil {
			utils.Error(fmt.Errorf("[ErrorQueue] API hit failed for CommId: %s: %v", data.CommId, err))
			return false
		}

		utils.Info(fmt.Sprintf("[ErrorQueue] API hit successful for CommId: %s", data.CommId))
	}

	// Step 3: Output Insertion (if needed)
	if doOutputInsertion && dbMappedData != nil {
		var outputTable string
		switch data.Channel {
		case variables.WhatsApp:
			outputTable = config.Configs.WhatsappOutputTable
		case variables.SMS:
			outputTable = config.Configs.SmsOutputTable
		case variables.Email:
			outputTable = config.Configs.EmailOutputTable
			delete(dbMappedData, "MobileNumber")
			dbMappedData["Email"] = data.Email
		case variables.RCS:
			outputTable = config.Configs.RcsOutputTable
		default:
			utils.Error(fmt.Errorf("[ErrorQueue] invalid channel: %s for CommId: %s", data.Channel, data.CommId))
			return false
		}

		if err := database.InsertData(outputTable, database.DBtechWrite, dbMappedData); err != nil {
			utils.Error(fmt.Errorf("[ErrorQueue] output insertion failed for CommId: %s: %v", data.CommId, err))
			// Don't send back to error queue to avoid infinite loops
			return false
		}

		utils.Info(fmt.Sprintf("[ErrorQueue] Output insertion successful for CommId: %s", data.CommId))
	}

	return true
}
