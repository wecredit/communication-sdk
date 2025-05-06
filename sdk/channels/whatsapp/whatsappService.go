package whatsapp

import (
	"encoding/json"
	"fmt"

	sinchWhatsapp "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/sinch"
	timesWhatsapp "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/times"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendWpByProcess(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	// var timeData extapimodels.TimesAPIModel
	// var sinchData extapimodels.SinchAPIModel

	requestBody := extapimodels.WhatsappRequestBody{
		Mobile:  msg.Mobile,
		Process: msg.ProcessName,
	}

	// timeData.Mobile = msg.Mobile
	// timeData.Process = msg.ProcessName

	// sinchData.Mobile = msg.Mobile
	// sinchData.Process = msg.ProcessName

	utils.Debug("Fetching whatsapp process data")
	wpProcessData, err := database.GetTemplateDetails(database.DBtech, msg.ProcessName, msg.Channel, msg.Vendor, msg.Stage)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while fetching WhatsApp process data for process '%s': %v", msg.ProcessName, err))
		return sdkModels.CommApiResponseBody{}, fmt.Errorf("error occurred while fetching WhatsApp process data for process '%s': %v", msg.ProcessName, err)
	}

	for _, record := range wpProcessData {
		if templateName, exists := record["TemplateName"]; exists && templateName != nil {
			// timeData.TemplateName = templateName.(string)
			// sinchData.TemplateName = templateName.(string)
			requestBody.TemplateName = templateName.(string)
		}
		if imageUrl, exists := record["ImageUrl"]; exists && imageUrl != nil {
			// timeData.ImageUrl = imageUrl.(string)
			requestBody.ImageUrl = imageUrl.(string)
		}
		if imageId, exists := record["ImageId"]; exists && imageId != nil {
			// sinchData.ImageID = imageId.(string)
			requestBody.ImageID = imageId.(string)
		}
		if buttonLink, exists := record["Link"]; exists && buttonLink != nil {
			// timeData.ButtonLink = buttonLink.(string)
			// sinchData.ButtonLink = buttonLink.(string)
			requestBody.ButtonLink = buttonLink.(string)
		}
	}

	var response extapimodels.WhatsappResponse

	// Hit Into WP
	switch msg.Vendor {
	case variables.TIMES:
		response = timesWhatsapp.HitTimesWhatsappApi(requestBody)
	case variables.SINCH:
		response = sinchWhatsapp.HitSinchWhatsappApi(requestBody)
	}

	response.CommId = msg.CommId
	response.TemplateName = requestBody.TemplateName
	response.Vendor = msg.Vendor

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}
	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("Whatsapp Response: %s", string(jsonBytes)))
	if response.IsSent {
		utils.Info(fmt.Sprintf("WhatsApp sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return sdkModels.CommApiResponseBody{
			CommId:  msg.CommId,
			Success: true,
		}, nil
	}

	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to send message for source: %s", msg.Vendor)
}
