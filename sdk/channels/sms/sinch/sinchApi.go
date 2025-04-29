package sms

import (
	"fmt"

	sinchpayloads "github.com/wecredit/communication-sdk/sdk/channels/sms/sinch/sinchPayloads"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchApi(msg sdkModels.CommApiRequestBody) (map[string]interface{}, error) {
	// var timeData extapimodels.TimesAPIModel
	var sinchData extapimodels.SinchSmsPayload

	sinchData.Mobile = msg.Mobile
	sinchData.Process = msg.ProcessName
	sinchData.Stage = msg.Stage

	utils.Debug("Fetching Sms Template Details")
	smsTemplateData, err := database.GetTemplateDetails(database.DBtech, msg.ProcessName, msg.Channel, msg.Vendor, msg.Stage)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while fetching SMS Template details for process '%s': %v", msg.ProcessName, err))
		return nil, fmt.Errorf("error occurred while fetching SMS Template details for process '%s': %v", msg.ProcessName, err)
	}

	for _, record := range smsTemplateData {
		if dltTemplateId, exists := record["DltTemplateId"]; exists && dltTemplateId != nil {
			sinchData.DltTemplateId = dltTemplateId.(int64)
		}
		if templateText, exists := record["TemplateText"]; exists && templateText != nil {
			sinchData.TemplateText = templateText.(string)
		}
	}

	// Getting the API URL
	apiUrl := config.Configs.SinchSmsApiUrl

	// Setting the API header
	apiHeader := map[string]string{
		"Content-Type": "application/json",
	}

	// Get api payload
	apiPayload, err := sinchpayloads.GetTemplatePayload(sinchData, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting SMS payload: %v", err))
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch SMS API: %v", err))
	}

	apiResponse["commId"] = msg.CommId

	fmt.Println("apiResponse:", apiResponse)

	// TODO Handling For Api Responses

	// if code, ok := apiResponse["code"].(float64); ok {
	// 	response.StatusCode = code
	// } else {
	// 	return response, fmt.Errorf("unexpected type for code: %T", apiResponse["code"])
	// }
	// response.Message = apiResponse["message"].(string)
	// response.Status = apiResponse["status"].(bool)

	return apiResponse, nil
}
