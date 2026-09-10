package sinchWhatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	sinchpayloads "github.com/wecredit/communication-sdk/internal/channels/whatsapp/sinch/sinchPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchWhatsappApi(sinchApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	if sinchApiModel.Client == variables.CreditSea {
		sinchApiModel.AppId = "creditseapd"
	} else {
		sinchApiModel.AppId = "wecreditpd"
	}

	accessToken, err := cache.GetSinchWhatsappAccessToken(sinchApiModel.Client)
	if err != nil {
		utils.Error(fmt.Errorf("sinch WA token: %v", err))
		responseBody.ResponseMessage = err.Error()
		return responseBody
	}
	sinchApiModel.AccessToken = accessToken

	// Legacy per-send token fetch kept for rollback (commented, not deleted):
	// headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	// generateTokenURL := config.Configs.SinchWhatsappTokenApiUrl
	// ... ApiHit token then set AccessToken ...

	responseBody = sendSinchWhatsappMessage(sinchApiModel)
	if isSinchWhatsappUnauthorized(responseBody) {
		cache.InvalidateSinchWhatsappToken(sinchApiModel.Client)
		accessToken, err = cache.GetSinchWhatsappAccessToken(sinchApiModel.Client)
		if err != nil {
			utils.Error(fmt.Errorf("sinch WA token refetch after 401: %v", err))
			responseBody.ResponseMessage = err.Error()
			return responseBody
		}
		sinchApiModel.AccessToken = accessToken
		responseBody = sendSinchWhatsappMessage(sinchApiModel)
	}
	return responseBody
}

func sendSinchWhatsappMessage(sinchApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	// Message-send RPS only (token path is outside this bucket — OQ-5 / Tushar).
	if err := ratelimit.WaitFor(context.Background(), ratelimit.Key(variables.SINCH, sinchApiModel.Client)); err != nil {
		responseBody.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		return responseBody
	}

	sendMessageURL := config.Configs.SinchWhatsappMessageApiUrl
	apiHeader := map[string]string{
		"Authorization": "Bearer " + sinchApiModel.AccessToken,
		"Content-Type":  "application/json",
	}

	apiPayload, err := getPayload(sinchApiModel)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	jsonBytes, _ := json.Marshal(apiPayload)
	utils.Debug(fmt.Sprintf("Sinch Whatsapp payload for mobile: %s and templateName: %s is: %s", sinchApiModel.Mobile, sinchApiModel.TemplateName, string(jsonBytes)))

	apiResponse, err := utils.ApiHit("POST", sendMessageURL, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Wp API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, sinchApiModel, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Sinch Wp API: %v", err)
		if status, ok := apiResponse["ApistatusCode"].(int); ok && status == 401 {
			responseBody.ResponseMessage = "unauthorized: " + responseBody.ResponseMessage
		}
		return responseBody
	}

	if status, ok := apiResponse["ApistatusCode"].(int); ok && status == 401 {
		responseBody.ResponseMessage = "unauthorized"
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

func isSinchWhatsappUnauthorized(resp extapimodels.WhatsappResponse) bool {
	return strings.Contains(strings.ToLower(resp.ResponseMessage), "unauthorized")
}

func getPayload(sinchApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(sinchApiModel.TemplateName, "utility") {
		fmt.Println("Generating Utility Payload for Sinch WhatsApp API")
		return sinchpayloads.GetSinchUtilityPayload(sinchApiModel), nil
	}
	return sinchpayloads.GetSinchMediaPayload(sinchApiModel), nil
}
