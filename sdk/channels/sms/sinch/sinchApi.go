package sms

import (
	"fmt"

	sinchpayloads "github.com/wecredit/communication-sdk/sdk/channels/sms/sinch/sinchPayloads"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchApi(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	var response sdkModels.CommApiResponseBody

	// Getting the API URL
	apiUrl := config.Configs.SinchSmsApiUrl

	// Setting the API header
	apiHeader := map[string]string{
		"Content-Type": "application/json",
	}

	// Get api payload
	apiPayload, err := sinchpayloads.GetTemplatePayload(msg.Mobile, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Wp API: %v", err))
	}

	fmt.Println("apiResponse:", apiResponse)

	// TODO Handling For Api Responses

	// if code, ok := apiResponse["code"].(float64); ok {
	// 	response.StatusCode = code
	// } else {
	// 	return response, fmt.Errorf("unexpected type for code: %T", apiResponse["code"])
	// }
	// response.Message = apiResponse["message"].(string)
	// response.Status = apiResponse["status"].(bool)

	return response, nil
}
