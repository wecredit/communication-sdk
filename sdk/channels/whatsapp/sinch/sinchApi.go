package sinchWhatsapp

import (
	"fmt"
	"strings"

	sinchpayloads "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/sinch/sinchPayloads"
	"github.com/wecredit/communication-sdk/sdk/config"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchWhatsappApi(sinchApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	// var response apiModels.WpApiResponseData
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	generateTokenURL := config.Configs.SinchTokenApiUrl
	if generateTokenURL == "" {
		utils.Error(fmt.Errorf("SINCH_GENERATE_TOKEN_API_URL is not set"))
		responseBody.ResponseMessage = "SINCH_GENERATE_TOKEN_API_URL is not set"
		return responseBody
	}

	var tokenPayload map[string]string
	if sinchApiModel.Client == variables.CreditSea { // For CreditSea, use the specific credentials
		tokenPayload = map[string]string{
			"grant_type": config.Configs.SinchGrantType,
			"client_id":  config.Configs.SinchClientId,
			"username":   config.Configs.CreditSeaSinchUsername,
			"password":   config.Configs.CreditSeaSinchPassword,
		}
		fmt.Println("TokenPayload:", tokenPayload)
		sinchApiModel.AppId = "creditseapd"
	} else {
		tokenPayload = map[string]string{
			"grant_type": config.Configs.SinchGrantType,
			"client_id":  config.Configs.SinchClientId,
			"username":   config.Configs.SinchUserName,
			"password":   config.Configs.SinchPassword,
		}
		sinchApiModel.AppId = "wecreditpd"
	}
	tokenResponse, err := utils.ApiHit("POST", generateTokenURL, headers, "", "", tokenPayload, variables.ContentTypeFormEncoded)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Generate Token API: %v", err))
	}

	if accessToken, ok := tokenResponse["access_token"].(string); ok {
		sinchApiModel.AccessToken = accessToken
	} else {
		responseBody.ResponseMessage = "failed to generate access token"
		return responseBody
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

	fmt.Println("WHatsapp payload:", apiPayload)

	apiResponse, err := utils.ApiHit("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
	}

	success := apiResponse["success"].(string)

	if success == "true" {
		responseBody.IsSent = true
		responseBody.ResponseMessage = "Message submitted successfully"
		responseBody.TransactionId = apiResponse["responseId"].(string)
	} else {
		responseBody.IsSent = false
		description, ok := apiResponse["description"].([]interface{})
		if ok && len(description) > 0 {
			firstDesc, ok := description[0].(map[string]interface{})
			if ok {
				errorCode, _ := firstDesc["errorCode"].(string)
				errorDesc, _ := firstDesc["errorDescription"].(string)
				responseBody.ResponseMessage = fmt.Sprintf("Error Code: %s, Description: %s", errorCode, errorDesc)
			}
		} else {
			responseBody.ResponseMessage = "failed to send message"
		}
	}

	fmt.Println("SINCH FINAL WHATSAPP RESPONSE:", responseBody)

	return responseBody
}

func getPayload(sinchApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(sinchApiModel.Process, "utility") {
		// For Utility Payload
		return sinchpayloads.GetSinchUtilityPayload(sinchApiModel), nil
	} else {
		return sinchpayloads.GetSinchMediaPayload(sinchApiModel), nil
	}
}
