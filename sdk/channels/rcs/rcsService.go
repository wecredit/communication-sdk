package rcs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	sinchRcs "github.com/wecredit/communication-sdk/sdk/channels/rcs/sinch"
	timesRcs "github.com/wecredit/communication-sdk/sdk/channels/rcs/times"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendRcsByProcess(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	requestBody := extapimodels.RcsRequesBody{
		Mobile:  msg.Mobile,
		Process: msg.ProcessName,
	}
	utils.Debug("Fetching rcs template data from cache")

	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return sdkModels.CommApiResponseBody{}, errors.New("template data not found in cache")
	}

	key := fmt.Sprintf("Process:%s|Stage:%s|Channel:%s|Vendor:%s", msg.ProcessName, strconv.Itoa(msg.Stage), msg.Channel, msg.Vendor)
	var data map[string]interface{}
	var ok bool
	if data, ok = templateDetails[key]; !ok {
		fmt.Println("No template found for the given key:", key)
		return sdkModels.CommApiResponseBody{}, fmt.Errorf("no template found for the given key: %s", key)
	}

	if templateName, exists := data["TemplateName"]; exists && templateName != nil {
		requestBody.TemplateName = templateName.(string)
	}
	if imageId, exists := data["ImageId"]; exists && imageId != nil {
		requestBody.AppId = imageId.(string)
	}

	utils.Debug("Fetching AppId data")
	rcsAppIdData, err := database.GetRcsAppId(database.DBtech, requestBody.AppId)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while fetching RCS APP Id data for app Id '%s': %v", requestBody.AppId, err))
		return sdkModels.CommApiResponseBody{}, fmt.Errorf("error occurred while fetching RCS APP Id data for app Id '%s': %v", requestBody.AppId, err)
	}

	if appIdKey, exists := rcsAppIdData["AppIdKey"]; exists && appIdKey != nil {
		requestBody.AppIdKey = appIdKey.(string)
	}
	if projectId, exists := rcsAppIdData["ProjectId"]; exists && projectId != nil {
		requestBody.ProjectId = projectId.(string)
	}
	if apikey, exists := rcsAppIdData["ProjectId"]; exists && apikey != nil {
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
