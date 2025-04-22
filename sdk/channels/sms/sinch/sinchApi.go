package sms

import (
	"fmt"

	sinchpayloads "github.com/wecredit/communication-sdk/sdk/channels/sms/sinch/sinchPayloads"
	"github.com/wecredit/communication-sdk/sdk/internal/models"
	apimodels "github.com/wecredit/communication-sdk/sdk/internal/models/apiModels"
	extapimodels "github.com/wecredit/communication-sdk/sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/internal/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchApi(timesApiModel extapimodels.TimesAPIModel, config models.Config) (apimodels.WpApiResponseData, error) {
	var response apimodels.WpApiResponseData

	// Getting the API URL
	apiUrl := config.SinchSmsApiUrl

	// Setting the API header
	apiHeader := map[string]string{
		"Content-Type": "application/json",
	}

	// Get api payload
	apiPayload, err := sinchpayloads.GetTemplatePayload("", config)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
	}

	// TODO Handling For Api Responses

	if code, ok := apiResponse["code"].(float64); ok {
		response.StatusCode = code
	} else {
		return response, fmt.Errorf("unexpected type for code: %T", apiResponse["code"])
	}
	response.Message = apiResponse["message"].(string)
	response.Status = apiResponse["status"].(bool)

	return response, nil
}
