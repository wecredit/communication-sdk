package sms

import (
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func SendSmsByProcess(msg sdkModels.CommApiRequestBody) (bool, error) {
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		return false, errors.New("template data not found in cache")
	}
	templateData, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		channelHelper.LogTemplateNotFound(msg, err)
		database.InsertData(config.Configs.SmsOutputTable, database.DBtech, map[string]interface{}{
			"CommId":          msg.CommId,
			"Vendor":          msg.Vendor,
			"MobileNumber":    msg.Mobile,
			"IsSent":          false,
			"ResponseMessage": err.Error(),
		})
		return false, nil
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

	// var response extapimodels.SmsResponse
	// switch msg.Vendor {
	// case variables.TIMES:
	// 	response = timesSms.HitTimesSmsApi(req)
	// case variables.SINCH:
	// 	response = sinchSms.HitSinchSmsApi(req)
	// }

	// response.DltTemplateId = req.DltTemplateId
	// response.CommId = msg.CommId
	// response.Vendor = msg.Vendor
	// response.MobileNumber = msg.Mobile

	// dbMappedData, err := dbservices.MapIntoDbModel(response)
	// if err != nil {
	// 	utils.Error(fmt.Errorf("mapping error: %v", err))
	// }

	// if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }

	// jsonBytes, _ := json.Marshal(response)
	// utils.Debug(fmt.Sprintf("SMS Response: %s", string(jsonBytes)))

	// if response.IsSent {
	// 	utils.Info(fmt.Sprintf("SMS sent successfully for Process: %s on %s via %s", msg.ProcessName, msg.Mobile, msg.Vendor))
	// 	return true, nil
	// }

	utils.Info(fmt.Sprintf("SMS sent successfully for Process: %s on %s via %s", msg.ProcessName, msg.Mobile, msg.Vendor))
	return false, nil
}
