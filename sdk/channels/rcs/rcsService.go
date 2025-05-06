package rcs

import (
	"encoding/json"
	"fmt"

	sinchRcs "github.com/wecredit/communication-sdk/sdk/channels/rcs/sinch"
	timesRcs "github.com/wecredit/communication-sdk/sdk/channels/rcs/times"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendRcsByProcess(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	// var timeData extapimodels.TimesAPIModel
	// var sinchData extapimodels.SinchAPIModel

	requestBody := extapimodels.RcsRequesBody{
		Mobile:  msg.Mobile,
		Process: msg.ProcessName,
	}

	// timeData.Mobile = msg.Mobile
	// timeData.Process = msg.ProcessName

	// sinchData.Mobile = msg.Mobile
	// sinchData.Process = msg.ProcessName

	utils.Debug("Fetching rcs template data")
	fmt.Println("Process:", msg.ProcessName, msg.Channel, msg.Vendor)
	rcsProcessData, err := database.GetTemplateDetails(database.DBtech, msg.ProcessName, msg.Channel, msg.Vendor, msg.Stage)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while fetching RCS process template for process '%s': %v", msg.ProcessName, err))
		return sdkModels.CommApiResponseBody{}, fmt.Errorf("error occurred while fetching RCS process template for process '%s': %v", msg.ProcessName, err)
	}

	for _, record := range rcsProcessData {
		if templateName, exists := record["TemplateName"]; exists && templateName != nil {
			// timeData.TemplateName = templateName.(string)
			// sinchData.TemplateName = templateName.(string)
			requestBody.TemplateName = templateName.(string)
		}
		if imageId, exists := record["ImageId"]; exists && imageId != nil {
			// sinchData.ImageID = imageId.(string)
			requestBody.AppId = imageId.(string)
		}
	}

	utils.Debug("Fetching AppId data")
	rcsAppIdData, err := database.GetRcsAppId(database.DBtech, requestBody.AppId)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while fetching RCS APP Id data for app Id '%s': %v", requestBody.AppId, err))
		return sdkModels.CommApiResponseBody{}, fmt.Errorf("error occurred while fetching RCS APP Id data for app Id '%s': %v", requestBody.AppId, err)
	}

	if appIdKey, exists := rcsAppIdData["AppIdKey"]; exists && appIdKey != nil {
		// timeData.TemplateName = templateName.(string)
		// sinchData.TemplateName = templateName.(string)
		requestBody.AppIdKey = appIdKey.(string)
	}
	if projectId, exists := rcsAppIdData["ProjectId"]; exists && projectId != nil {
		// sinchData.ImageID = imageId.(string)
		requestBody.ProjectId = projectId.(string)
	}
	if apikey, exists := rcsAppIdData["ProjectId"]; exists && apikey != nil {
		// sinchData.ImageID = imageId.(string)
		requestBody.ApiKey = apikey.(string)
	}

	var response extapimodels.RcsResponse

	// Hit Into WP
	switch msg.Vendor {
	case variables.TIMES:
		response = timesRcs.HitTimesRcsApi(requestBody)
	case variables.SINCH:
		response = sinchRcs.HitSinchRcsApi(requestBody)
	}

	response.CommId = msg.CommId
	response.TemplateName = requestBody.TemplateName
	response.Vendor = msg.Vendor

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	if err := database.InsertData(config.Configs.RcsOutputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}
	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("RCS Response: %s", string(jsonBytes)))
	if response.IsSent {
		utils.Info(fmt.Sprintf("RCS sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return sdkModels.CommApiResponseBody{
			CommId:  msg.CommId,
			Success: true,
		}, nil
	}

	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to send message for source: %s", msg.Vendor)
}
