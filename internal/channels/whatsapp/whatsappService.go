package whatsapp

import (
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	channelHelper "github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func SendWpByProcess(msg sdkModels.CommApiRequestBody) (bool, error) {
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
		return false, errors.New("template data not found in cache")
	}

	data, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		channelHelper.LogTemplateNotFound(msg, err)
		if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, map[string]interface{}{
			"CommId":          msg.CommId,
			"Vendor":          msg.Vendor,
			"MobileNumber":    msg.Mobile,
			"IsSent":          false,
			"ResponseMessage": fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and Vendor: %s", msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor),
		}); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
			return false, nil
		}
		return false, nil
	}
	msg.Vendor = matchedVendor

	channelHelper.PopulateWhatsappFields(&requestBody, data)

	// var response extapimodels.WhatsappResponse

	// // Hit Into WP
	// switch msg.Vendor {
	// case variables.TIMES:
	// 	response = timesWhatsapp.HitTimesWhatsappApi(requestBody)
	// case variables.SINCH:
	// 	response = sinchWhatsapp.HitSinchWhatsappApi(requestBody)
	// }

	// response.CommId = msg.CommId
	// response.TemplateName = requestBody.TemplateName
	// response.Vendor = msg.Vendor
	// response.MobileNumber = msg.Mobile

	// dbMappedData, err := services.MapIntoDbModel(response)
	// if err != nil {
	// 	utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	// }

	// if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }
	// jsonBytes, _ := json.Marshal(response)
	// utils.Debug(fmt.Sprintf("Whatsapp Response: %s", string(jsonBytes)))
	// if response.IsSent {
	// 	utils.Info(fmt.Sprintf("WhatsApp sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
	// 	if msg.Client == variables.CreditSea {
	// 		redis.IncrementCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount)
	// 	}
	// 	return true, nil
	// }
	utils.Info(fmt.Sprintf("WhatsApp sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
	return false, nil
}
