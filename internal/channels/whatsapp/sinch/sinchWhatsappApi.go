package sinchWhatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// sinchAPIResponse is the Sinch WA send contract (success is the string "true"/"false").
type sinchAPIResponse struct {
	Success     string `json:"success"`
	ResponseID  string `json:"responseId"`
	Description []struct {
		ErrorCode        string `json:"errorCode"`
		ErrorDescription string `json:"errorDescription"`
	} `json:"description"`
}

func HitSinchWhatsappApi(sinchApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	// AppId must come from TemplateDetails (PopulateWhatsappFields). Stale Client
	// hardcodes (wecreditpd / creditseapd) removed — live WeCredit Sinch uses wecreditpd4.
	if strings.TrimSpace(sinchApiModel.AppId) == "" {
		responseBody.ResponseMessage = "sinch AppId missing from template details"
		return responseBody
	}

	accessToken, err := cache.GetSinchWhatsappAccessToken(sinchApiModel.Client)
	if err != nil {
		utils.Error(fmt.Errorf("sinch WA token: %v", err))
		responseBody.ResponseMessage = err.Error()
		return responseBody
	}
	sinchApiModel.AccessToken = accessToken

	// Hermis mints a token per send with no 401 retry. SDK caches tokens and
	// refetches once on HTTP 401 only (explicit status, not message substring).
	responseBody, unauthorized := sendSinchWhatsappMessage(sinchApiModel)
	if unauthorized {
		cache.InvalidateSinchWhatsappToken(sinchApiModel.Client)
		accessToken, err = cache.GetSinchWhatsappAccessToken(sinchApiModel.Client)
		if err != nil {
			utils.Error(fmt.Errorf("sinch WA token refetch after 401: %v", err))
			responseBody.ResponseMessage = err.Error()
			return responseBody
		}
		sinchApiModel.AccessToken = accessToken
		responseBody, _ = sendSinchWhatsappMessage(sinchApiModel)
	}
	return responseBody
}

func sendSinchWhatsappMessage(sinchApiModel extapimodels.WhatsappRequestBody) (extapimodels.WhatsappResponse, bool) {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	// Message-send RPS only (token path is outside this bucket — OQ-5 / Tushar).
	if err := ratelimit.WaitFor(context.Background(), ratelimit.KeyWithChannel(variables.SINCH, sinchApiModel.Client, "whatsapp")); err != nil {
		responseBody.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		return responseBody, false
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
	responseBody.RawPayload = string(jsonBytes)
	utils.Debug(fmt.Sprintf("Sinch Whatsapp payload for mobile: %s and templateName: %s is: %s", sinchApiModel.Mobile, sinchApiModel.TemplateName, string(jsonBytes)))

	var apiResponse sinchAPIResponse
	statusCode, rawBody, err := utils.ApiHitJSON("POST", sendMessageURL, apiHeader, "", "", apiPayload, variables.ContentTypeJSON, &apiResponse)
	if rawBody != "" {
		responseBody.RawResponse = rawBody
	}

	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Wp API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, sinchApiModel, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Sinch Wp API: %v", err)
		return responseBody, statusCode == http.StatusUnauthorized
	}

	if statusCode == http.StatusUnauthorized {
		responseBody.ResponseMessage = "unauthorized"
		return responseBody, true
	}

	if strings.TrimSpace(apiResponse.Success) == "" {
		utils.Error(fmt.Errorf("success field is missing in Sinch WA API response"))
		responseBody.ResponseMessage = "failed to send message due to missing success field"
		return responseBody, false
	}

	if apiResponse.Success == "true" {
		responseBody.IsSent = true
		responseBody.ResponseMessage = "Message submitted successfully"
		responseBody.TransactionId = strings.TrimSpace(apiResponse.ResponseID)
	} else {
		responseBody.IsSent = false
		if len(apiResponse.Description) > 0 {
			responseBody.ResponseMessage = fmt.Sprintf("Error Code: %s, Description: %s",
				apiResponse.Description[0].ErrorCode, apiResponse.Description[0].ErrorDescription)
		} else {
			responseBody.ResponseMessage = "failed to send message"
		}
	}

	fmt.Println("SINCH FINAL WHATSAPP RESPONSE:", responseBody)
	return responseBody, false
}

func getPayload(sinchApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(sinchApiModel.TemplateName, "utility") {
		fmt.Println("Generating Utility Payload for Sinch WhatsApp API")
		return sinchpayloads.GetSinchUtilityPayload(sinchApiModel), nil
	}
	return sinchpayloads.GetSinchMediaPayload(sinchApiModel), nil
}
