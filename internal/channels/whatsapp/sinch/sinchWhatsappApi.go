package sinchWhatsapp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	sinchpayloads "github.com/wecredit/communication-sdk/internal/channels/whatsapp/sinch/sinchPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchWhatsappApi(sinchApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	// var response apiModels.WpApiResponseData
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	generateTokenURL := config.Configs.SinchWhatsappTokenApiUrl
	if generateTokenURL == "" {
		utils.Error(fmt.Errorf("SINCH_GENERATE_TOKEN_API_URL is not set"))
		responseBody.ResponseMessage = "SINCH_GENERATE_TOKEN_API_URL is not set"
		return responseBody
	}

	var tokenPayload map[string]string
	if sinchApiModel.Client == variables.CreditSea { // For CreditSea, use the specific credentials
		tokenPayload = map[string]string{
			"grant_type": config.Configs.SinchWhatsappGrantType,
			"client_id":  config.Configs.SinchWhatsappClientId,
			"username":   config.Configs.CreditSeaSinchWhatsappUsername,
			"password":   config.Configs.CreditSeaSinchWhatsappPassword,
		}
		sinchApiModel.AppId = "creditseapd"
	} else {
		tokenPayload = map[string]string{
			"grant_type": config.Configs.SinchWhatsappGrantType,
			"client_id":  config.Configs.SinchWhatsappClientId,
			"username":   config.Configs.SinchWhatsappUserName,
			"password":   config.Configs.SinchWhatsappPassword,
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

	sendMessageURL := config.Configs.SinchWhatsappMessageApiUrl

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

	// fmt.Println("WHatsapp payload:", apiPayload)

	jsonBytes, _ := json.Marshal(apiPayload)
	utils.Debug(fmt.Sprintf("Sinch Whatsapp payload for mobile: %s and templateName: %s is: %s", sinchApiModel.Mobile, sinchApiModel.TemplateName, string(jsonBytes)))

	apiResponse, err := utils.ApiHit("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Wp API: %v", err))
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Sinch Wp API: %v", err)
		return responseBody
	}

	success, ok := apiResponse["success"].(string)
	if !ok {
		utils.Error(fmt.Errorf("success field is missing or not a string in API response"))
		responseBody.IsSent = false
		responseBody.ResponseMessage = "failed to send message due to missing success field"
		return responseBody
	}

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
	if strings.Contains(sinchApiModel.TemplateName, "utility") {
		// For Utility Payload
		fmt.Println("Generating Utility Payload for Sinch WhatsApp API")
		return sinchpayloads.GetSinchUtilityPayload(sinchApiModel), nil
	} else {
		return sinchpayloads.GetSinchMediaPayload(sinchApiModel), nil
	}
}
