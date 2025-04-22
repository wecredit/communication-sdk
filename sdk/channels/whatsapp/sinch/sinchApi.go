package services

import (
	"fmt"
	"strings"

	sinchpayloads "dev.azure.com/wctec/communication-engine/sdk/channels/whatsapp/sinch/sinchPayloads"
	"dev.azure.com/wctec/communication-engine/sdk/config"
	"dev.azure.com/wctec/communication-engine/sdk/internal/models/apiModels"
	extapimodels "dev.azure.com/wctec/communication-engine/sdk/internal/models/extApiModels"
	"dev.azure.com/wctec/communication-engine/sdk/internal/utils"
	"dev.azure.com/wctec/communication-engine/sdk/variables"
)

func HitSinchApi(sinchApiModel extapimodels.SinchAPIModel) apiModels.WpApiResponseData {
	var response apiModels.WpApiResponseData

	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	generateTokenURL := config.Configs.SinchTokenApiUrl
	if generateTokenURL == "" {
		utils.Error(fmt.Errorf("SINCH_GENERATE_TOKEN_API_URL is not set"))
	}

	tokenPayload := map[string]string{
		"grant_type": config.Configs.SinchGrantType,
		"client_id":  config.Configs.SinchClientId,
		"username":   config.Configs.SinchUserName,
		"password":   config.Configs.SinchPassword,
	}

	tokenResponse, err := utils.ApiHit("POST", generateTokenURL, headers, "", "", tokenPayload, variables.ContentTypeFormEncoded)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Generate Token API: %v", err))
	}

	if accessToken, ok := tokenResponse["access_token"].(string); ok {
		sinchApiModel.AccessToken = accessToken
	} else {
		response.StatusCode = 500
		response.Message = "failed to generate access token"
		response.Status = false
		return response
	}

	sendMessageURL := config.Configs.SinchMessageApiUrl

	// Getting the API URL
	apiUrl := sendMessageURL

	// Setting the API header
	apiHeader := map[string]string{
		"Authorization": "Bearer " + sinchApiModel.AccessToken,
		"Content-Type":  "application/json",
	}

	// Get api payload
	apiPayload, err := getPayload(sinchApiModel)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	apiResponse, err := utils.ApiHit("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
	}

	// TODO Handling For Api Responses

	success := apiResponse["success"].(string)

	if success == "true" {
		response.StatusCode = 200
		response.Message = "success"
		response.Status = true
	} else {
		response.StatusCode = 500
		response.Message = "failed to send message"
		response.Status = false
	}

	return response
}

func getPayload(sinchApiModel extapimodels.SinchAPIModel) (map[string]interface{}, error) {
	if strings.Contains(sinchApiModel.Process, "utility") {
		// For Utility Payload
		return sinchpayloads.GetSinchUtilityPayload(sinchApiModel), nil
	} else {
		return sinchpayloads.GetSinchMediaPayload(sinchApiModel), nil
	}
}
