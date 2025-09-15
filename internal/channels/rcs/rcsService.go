package rcs

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	sinchRcs "github.com/wecredit/communication-sdk/internal/channels/rcs/sinch"
	timesRcs "github.com/wecredit/communication-sdk/internal/channels/rcs/times"
	"github.com/wecredit/communication-sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	dbservices "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendRcsByProcess(msg sdkModels.CommApiRequestBody) (bool, error) {
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		return false, errors.New("template data not found in cache")
	}
	templateData, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		channelHelper.LogTemplateNotFound(msg, err)
		return true, nil // message processed but not sent as Template not found
	}
	msg.Vendor = matchedVendor

	req := extapimodels.RcsRequestBody{
		Mobile:  msg.Mobile,
		Process: msg.ProcessName,
	}
	channelHelper.PopulateRcsFields(&req, templateData)

	rcsAppIdData, err := database.GetRcsAppId(database.DBtech, req.AppId)
	if err != nil {
		utils.Error(fmt.Errorf("failed to fetch RCS AppId data: %v", err))
		return false, fmt.Errorf("failed to fetch RCS AppId data: %v", err)
	}

	if val, ok := rcsAppIdData["AppIdKey"].(string); ok {
		req.AppIdKey = val
	}
	if val, ok := rcsAppIdData["ProjectId"].(string); ok {
		req.ProjectId = val
		req.ApiKey = val
	}

	var response extapimodels.RcsResponse
	// Check if the vendor should be hit
	shouldHitVendor := channelHelper.ShouldHitVendor(msg.Client, msg.Channel)

	if shouldHitVendor {
		switch msg.Vendor {
		case variables.TIMES:
			response = timesRcs.HitTimesRcsApi(req)
		case variables.SINCH:
			response = sinchRcs.HitSinchRcsApi(req)
		}
	}

	response.CommId = msg.CommId
	response.TemplateName = req.TemplateName
	response.Vendor = msg.Vendor

	dbMappedData, err := dbservices.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("mapping error: %v", err))
	}
	database.InsertData(config.Configs.RcsOutputTable, database.DBtech, dbMappedData)

	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("RCS Response: %s", string(jsonBytes)))

	if response.IsSent {
		utils.Info(fmt.Sprintf("RCS sent successfully for Process: %s on %s via %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return true, nil
	}
	return true, nil // message processed but not sent as response.IsSent is false
}
