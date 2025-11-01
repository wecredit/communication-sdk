package sms

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	sinchSms "github.com/wecredit/communication-sdk/internal/channels/sms/sinch"
	timesSms "github.com/wecredit/communication-sdk/internal/channels/sms/times"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendSmsByProcess(msg sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		return false, nil, errors.New("template data not found in cache")
	}
	templateData, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		return channelHelper.HandleTemplateNotFoundError(msg, err)
	}

	msg.Vendor = matchedVendor

	req := extapimodels.SmsRequestBody{
		Mobile:            msg.Mobile,
		Process:           msg.ProcessName,
		Client:            msg.Client,
		EmiAmount:         msg.EmiAmount,
		CustomerName:      msg.CustomerName,
		LoanId:            msg.LoanId,
		ApplicationNumber: msg.ApplicationNumber,
		DueDate:           msg.DueDate,
		Description:       msg.Description,
	}
	channelHelper.PopulateSmsFields(&req, templateData)

	var response extapimodels.SmsResponse

	// Check if the vendor should be hit
	shouldHitVendor := channelHelper.ShouldHitVendor(msg.Client, msg.Channel)
	if shouldHitVendor {
		switch msg.Vendor {
		case variables.TIMES:
			response, err = timesSms.HitTimesSmsApi(req)
			if err != nil {
				utils.Error(fmt.Errorf("error in hitting into times sms api: %v", err))
				return false, nil, fmt.Errorf("error in hitting into times sms api: %v", err)
			}
		case variables.SINCH:
			response, err = sinchSms.HitSinchSmsApi(req)
			if err != nil {
				utils.Error(fmt.Errorf("error in hitting into sinch sms api: %v", err))
				return false, nil, fmt.Errorf("error in hitting into sinch sms api: %v", err)
			}
		}
	}

	// Step 2: Once you have responseId, update the value of transactionId in redis
	if err := channelHelper.UpdateRedisTransactionId(msg.Mobile, msg.Channel, msg.Stage, response.TransactionId); err != nil {
		utils.Error(fmt.Errorf("failed to update Redis transactionId: %v", err))
	}

	response.DltTemplateId = req.DltTemplateId
	response.CommId = msg.CommId
	response.Vendor = msg.Vendor
	response.MobileNumber = msg.Mobile

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("mapping error: %v", err))
	}

	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("SMS Response: %s", string(jsonBytes)))

	if shouldHitVendor && response.IsSent {
		utils.Info(fmt.Sprintf("SMS sent successfully for Process: %s on %s via %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return true, dbMappedData, nil
	}

	if !shouldHitVendor {
		// Step 2: Once you have error message, update the error message in redis
		if err := channelHelper.HandleShouldHitVendorOffError(msg.Mobile, msg.Channel, msg.Stage); err != nil {
			utils.Error(fmt.Errorf("failed to handle shouldHitVendor off error: %v", err))
		}
	}

	utils.Info(fmt.Sprintf("SMS sent successfully for Process: %s on %s via %s", msg.ProcessName, msg.Mobile, msg.Vendor))
	return true, dbMappedData, nil
	// if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }
}
