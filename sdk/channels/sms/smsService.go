package sms

import (
	"encoding/json"
	"fmt"

	sinchSms "github.com/wecredit/communication-sdk/sdk/channels/sms/sinch"
	timesSms "github.com/wecredit/communication-sdk/sdk/channels/sms/times"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendSmsByProcess(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {

	requestBody := extapimodels.SmsRequestBody{
		Mobile:  msg.Mobile,
		Process: msg.ProcessName,
	}

	// var responseData extapimodels.SmsResponse
	// var timeData extapimodels.TimesAPIModel
	// var sinchData extapimodels.SinchSmsPayload

	// sinchData.Mobile = msg.Mobile
	// sinchData.Process = msg.ProcessName
	// sinchData.Stage = msg.Stage
	// sinchData.CommId = msg.CommId

	// timeData.Mobile = msg.Mobile
	// timeData.Process = msg.ProcessName
	// timeData.Stage = msg.Stage
	// timeData.CommId = msg.CommId

	utils.Debug("Fetching SMS process data")
	smsProcessData, err := database.GetTemplateDetails(database.DBtech, msg.ProcessName, msg.Channel, msg.Vendor, msg.Stage)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while fetching SMS process data for process '%s': %v", msg.ProcessName, err))
		return sdkModels.CommApiResponseBody{}, fmt.Errorf("error occurred while fetching WhatsApp process data for process '%s': %v", msg.ProcessName, err)
	}

	for _, record := range smsProcessData {
		if dltTemplateId, exists := record["DltTemplateId"]; exists && dltTemplateId != nil {
			// sinchData.DltTemplateId = dltTemplateId.(int64)
			// timeData.DltTemplateId = dltTemplateId.(int64)
			requestBody.DltTemplateId = dltTemplateId.(int64)
		}
		if templateText, exists := record["TemplateText"]; exists && templateText != nil {
			// sinchData.TemplateText = templateText.(string)
			// timeData.TemplateText = templateText.(string)
			requestBody.TemplateText = templateText.(string)
		}
	}

	var response extapimodels.SmsResponse
	// Hit Into WP
	switch msg.Vendor {
	case variables.TIMES:
		response = timesSms.HitTimesSmsApi(requestBody)
	case variables.SINCH:
		response = sinchSms.HitSinchSmsApi(requestBody)
	}
	response.DltTemplateId = requestBody.DltTemplateId
	response.CommId = msg.CommId
	response.Vendor = msg.Vendor

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}

	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("SmsResponse: %s", string(jsonBytes)))
	if response.IsSent {
		utils.Info(fmt.Sprintf("SMS sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return sdkModels.CommApiResponseBody{
			CommId:  msg.CommId,
			Success: true,
		}, nil
	}

	/* 	// Hit Into WP
	   	switch msg.Vendor {
	   	case variables.TIMES:
	   		timeResponse := timesSms.HitTimesSmsApi(requestBody)
	   		jsonBytes, _ := json.Marshal(timeResponse)
	   		utils.Debug(fmt.Sprintf("TimesSmsResponse: %s", string(jsonBytes)))

	   		if timeResponse.IsSent {
	   			utils.Info(fmt.Sprintf("SMS sent successfully for: %s", msg.Mobile))
	   			return sdkModels.CommApiResponseBody{
	   				CommId:  msg.CommId,
	   				Success: true,
	   			}, nil
	   		}

	   	case variables.SINCH:
	   		sinchResponse, _ := sinchSms.HitSinchSmsApi(requestBody)
	   		jsonBytes, _ := json.Marshal(sinchResponse)
	   		utils.Debug(fmt.Sprintf("SinchSmsResponse: %s", string(jsonBytes)))
	   		if sinchResponse.IsSent {
	   			utils.Info(fmt.Sprintf("SMS sent successfully for: %s", msg.Mobile))
	   			return sdkModels.CommApiResponseBody{
	   				CommId:  msg.CommId,
	   				Success: true,
	   			}, nil
	   		}
	   	} */
	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to send message for process: %s", msg.ProcessName)
}
