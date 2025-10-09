package sms

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/redis"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func SendSmsByProcess(msg sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		return false, nil, errors.New("template data not found in cache")
	}
	templateData, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		channelHelper.LogTemplateNotFound(msg, err)
		redisKey := fmt.Sprintf("%s_%s", msg.Mobile, strings.ToUpper(msg.Channel))
		err = redis.UpdateMobileChannelValue(redis.RDB, config.Configs.CommIdempotentKey, redisKey, "template not found")
		if err != nil {
			utils.Error(fmt.Errorf("redis update value failed: %v", err))
		}

		dbResponse := map[string]interface{}{
			"CommId":          msg.CommId,
			"Vendor":          msg.Vendor,
			"MobileNumber":    msg.Mobile,
			"IsSent":          false,
			"ResponseMessage": fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and Vendor: %s", msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor),
		}
		return true, dbResponse, nil // message processed but not sent as Template not found
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
	utils.Debug(fmt.Sprintf("Channel: %s Mobile: %s, Should hit vendor: %v\n", msg.Channel, msg.Mobile, shouldHitVendor))
	// if shouldHitVendor {
	// 	switch msg.Vendor {
	// 	case variables.TIMES:
	// 		response = timesSms.HitTimesSmsApi(req)
	// 	case variables.SINCH:
	// 		response = sinchSms.HitSinchSmsApi(req)
	// 	}
	// }

	if shouldHitVendor{
		response.TransactionId = fmt.Sprintf("shouldHitVendor is on for mobile %s", msg.Mobile)
	}

	// Step 2: Once you have responseId, update the value
	redisKey := fmt.Sprintf("%s_%s", msg.Mobile, strings.ToUpper(msg.Channel))
	err = redis.UpdateMobileChannelValue(redis.RDB, config.Configs.CommIdempotentKey, redisKey, response.TransactionId)
	if err != nil {
		utils.Error(fmt.Errorf("redis update value failed: %v", err))
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
		// Step 2: Once you have responseId, update the value
		redisKey := fmt.Sprintf("%s_%s", msg.Mobile, strings.ToUpper(msg.Channel))
		response.TransactionId = fmt.Sprintf("shouldHitVendor is off for mobile %s", msg.Mobile)
		dbMappedData["TransactionId"] = response.TransactionId
		err = redis.UpdateMobileChannelValue(redis.RDB, config.Configs.CommIdempotentKey, redisKey, response.TransactionId)
		if err != nil {
			utils.Error(fmt.Errorf("redis update value failed: %v", err))
		}
	}

	utils.Info(fmt.Sprintf("SMS sent successfully for Process: %s on %s via %s", msg.ProcessName, msg.Mobile, msg.Vendor))
	return true, dbMappedData, nil
	// if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }
}
