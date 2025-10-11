package services

import (
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func CheckIfDataAlreadyExists(data sdkModels.CommApiRequestBody, redisKey string, redisKeyVal string) (bool, error) {
	// check if record already exists in output table
	// if exists, return true
	// else insert in database and return false

	var _, outputTableName string
	switch data.Channel {
	case variables.WhatsApp:
		_ = config.Configs.SdkWhatsappInputTable // input table
		outputTableName = config.Configs.WhatsappOutputTable
	case variables.SMS:
		_ = config.Configs.SdkSmsInputTable
		outputTableName = config.Configs.SmsOutputTable
	case variables.Email:
		_ = config.Configs.SdkEmailInputTable
		outputTableName = config.Configs.EmailOutputTable
	default:
		return false, fmt.Errorf("invalid channel: %s", data.Channel)
	}

	var exists bool
	var err error

	// TODO: add logic to check if record exists in input table

	// check if record already exists in output table
	if exists, err = database.CheckIfRecordAlreadyExists(outputTableName, data.Mobile, redisKeyVal); err != nil {
		return false, fmt.Errorf("error checking if record exists in output table: %w", err)
	}

	if exists {
		utils.Debug(fmt.Sprintf("record already exists in output table for mobile: %s, transactionId: %s", data.Mobile, redisKeyVal))
		return true, nil
	} else {
		dbResponse := map[string]interface{}{
			"CommId":          data.CommId,
			"TransactionId":   redisKeyVal,
			"MobileNumber":    data.Mobile,
			"IsSent":          true,
			"ResponseMessage": "Message submitted successfully",
		}
		if err := database.InsertData(outputTableName, database.DBtechWrite, dbResponse); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
			dbResponse["tableName"] = outputTableName
			if queueErr := queue.SendMessageWithSubject(queue.SQSClient, dbResponse, config.Configs.AwsErrorQueueUrl, variables.OutputInsertionFails, err.Error()); queueErr != nil {
				utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
				return false, fmt.Errorf("error sending message to error queue: %w", queueErr)
			}
		}
		return false, nil
	}
}
