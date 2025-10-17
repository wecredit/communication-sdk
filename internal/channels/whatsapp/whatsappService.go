package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	channelHelper "github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	sinchWhatsapp "github.com/wecredit/communication-sdk/internal/channels/whatsapp/sinch"
	timesWhatsapp "github.com/wecredit/communication-sdk/internal/channels/whatsapp/times"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/redis"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendWpByProcess(msg sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
	requestBody := extapimodels.WhatsappRequestBody{
		Mobile:            msg.Mobile,
		Process:           msg.ProcessName,
		Client:            msg.Client,
		EmiAmount:         msg.EmiAmount,
		CustomerName:      msg.CustomerName,
		LoanId:            msg.LoanId,
		ApplicationNumber: msg.ApplicationNumber,
		DueDate:           msg.DueDate,
	}

	utils.Debug("Fetching WHATSAPP process data from cache")
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return false, nil, errors.New("template data not found in cache")
	}

	data, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		return channelHelper.HandleTemplateNotFoundError(msg, err)
	}

	msg.Vendor = matchedVendor

	channelHelper.PopulateWhatsappFields(&requestBody, data)

	var response extapimodels.WhatsappResponse

	// Check if the vendor should be hit
	shouldHitVendor := channelHelper.ShouldHitVendor(msg.Client, msg.Channel)
	utils.Debug(fmt.Sprintf("Channel: %s Mobile: %s, Should hit vendor: %v\n", msg.Channel, msg.Mobile, shouldHitVendor))

	if shouldHitVendor {
		// Hit Into WP
		switch msg.Vendor {
		case variables.TIMES:
			response = timesWhatsapp.HitTimesWhatsappApi(requestBody)
		case variables.SINCH:
			response = sinchWhatsapp.HitSinchWhatsappApi(requestBody)
		}
	}

	// apihit. : successful -> redis

	// unsuccessful -> error

	// delete message then insert in the database.

	// Step 2: Once you have responseId, update the value of transactionId in redis
	if err := channelHelper.UpdateRedisTransactionId(msg.Mobile, msg.Channel, msg.Stage, response.TransactionId); err != nil {
		utils.Error(fmt.Errorf("failed to update Redis transactionId: %v", err))
	}

	response.CommId = msg.CommId
	response.TemplateName = requestBody.TemplateName
	response.Vendor = msg.Vendor
	response.MobileNumber = msg.Mobile

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}
	
	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("Whatsapp Response: %s", string(jsonBytes)))
	if shouldHitVendor && response.IsSent {
		utils.Info(fmt.Sprintf("WhatsApp sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		if msg.Client == variables.CreditSea {
			redis.IncrementCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount)
		}
		return true, dbMappedData, nil
	}
	
	if !shouldHitVendor {
		// Step 2: Once you have error message, update the error message in redis
		dbMappedData["ResponseMessage"] = "shouldHitVendor is off for mobile " + msg.Mobile
		if err := channelHelper.HandleShouldHitVendorOffError(msg.Mobile, msg.Channel, msg.Stage); err != nil {
			utils.Error(fmt.Errorf("failed to handle shouldHitVendor off error: %v", err))
		}
	}

	utils.Info(fmt.Sprintf("WhatsApp not sent for Process: %s on %s through %s as shouldHitVendor is false or response.IsSent is false", msg.ProcessName, msg.Mobile, msg.Vendor))
	return true, dbMappedData, nil // message processed but not sent as shouldHitVendor is false or response.IsSent is false

	// if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }
}
