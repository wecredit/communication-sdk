package pinnacleWhatsapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	pinnaclepayloads "github.com/wecredit/communication-sdk/internal/channels/whatsapp/pinnacle/pinnaclePayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitPinnacleWhatsappApi(pinnacleApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	// var response apiModels.WpApiResponseData
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	sendMessageURL := config.Configs.PinnacleWhatsappMessageApiUrl
	if sendMessageURL == "" {
		utils.Error(fmt.Errorf("PINNACLE_WHATSAPP_MESSAGE_API_URL is not set"))
		responseBody.ResponseMessage = "PINNACLE_WHATSAPP_MESSAGE_API_URL is not set"
		return responseBody
	}

	// Getting the API URL
	apiUrl := sendMessageURL

	// Setting the API header
	apiHeader := map[string]string{
		"apikey":       config.Configs.PinnacleWhatsappApiKey,
		"Content-Type": "application/json",
	}

	// Get api payload
	apiPayload, err := getPayload(pinnacleApiModel)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	fmt.Println("Pinnacle Whatsapp payload:", apiPayload)

	jsonBytes, _ := json.Marshal(apiPayload)
	utils.Debug(fmt.Sprintf("Pinnacle Whatsapp payload for mobile: %s and templateName: %s is: %s", pinnacleApiModel.Mobile, pinnacleApiModel.TemplateName, string(jsonBytes)))

	apiResponse, err := utils.ApiHit("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Pinnacle Wp API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, pinnacleApiModel, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Pinnacle Wp API: %v", err)
		return responseBody
	}

	statusCode := apiResponse["ApistatusCode"].(int)
	if statusCode == http.StatusOK {
		responseBody.IsSent = true
		responseBody.ResponseMessage = "Message submitted successfully"
		if msgID, ok := pinnacleWhatsappMessageID(apiResponse); ok {
			responseBody.TransactionId = msgID
		}
	} else {
		responseBody.IsSent = false
		responseBody.ResponseMessage = pinnacleWhatsappErrorBodyMessage(apiResponse)
	}

	fmt.Println("PINNACLE FINAL WHATSAPP RESPONSE:", responseBody)
	responseBody.ResponseMessage = responseBody.ResponseMessage + " | " + apiPayload["biz_opaque_callback_data"].(map[string]interface{})["lead_id"].(string)
	return responseBody
}

func pinnacleWhatsappMessageID(apiResponse map[string]interface{}) (string, bool) {
	messages, ok := apiResponse["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		return "", false
	}
	first, ok := messages[0].(map[string]interface{})
	if !ok {
		return "", false
	}
	id, ok := first["id"].(string)
	if !ok || strings.TrimSpace(id) == "" {
		return "", false
	}
	return id, true
}

// pinnacleWhatsappErrorBodyMessage parses 4xx/5xx JSON bodies: nested `error` (OAuth) or top-level failed payload.
func pinnacleWhatsappErrorBodyMessage(apiResponse map[string]interface{}) string {
	if errObj, ok := apiResponse["error"].(map[string]interface{}); ok {
		return formatPinnacleMessageAndDetails(
			errObj["message"].(string),
			pinnacleErrorDataDetails(errObj),
		)
	}
	if status, _ := apiResponse["status"].(string); status == "failed" {
		details := ""
		if data, ok := apiResponse["data"].(map[string]interface{}); ok {
			details = data["details"].(string)
		}
		return formatPinnacleMessageAndDetails(apiResponse["message"].(string), details)
	}
	return "failed to send message"
}

func pinnacleErrorDataDetails(errObj map[string]interface{}) string {
	ed, ok := errObj["error_data"].(map[string]interface{})
	if !ok {
		return ""
	}
	return ed["details"].(string)
}

func formatPinnacleMessageAndDetails(message, details string) string {
	message = strings.TrimSpace(message)
	details = strings.TrimSpace(details)
	if message == "" && details == "" {
		return "failed to send message"
	}
	if details == "" {
		return message
	}
	if message == "" {
		return details
	}
	return fmt.Sprintf("%s, details: %s", message, details)
}

func getPayload(pinnacleApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(pinnacleApiModel.TemplateName, "utility") || strings.Contains(pinnacleApiModel.TemplateName, "marketing") {
		// For Utility Payload
		return pinnaclepayloads.GetPinnacleUtilityPayload(pinnacleApiModel), nil
	} else if strings.Contains(pinnacleApiModel.TemplateName, "media") {
		return pinnaclepayloads.GetPinnacleMediaPayload(pinnacleApiModel), nil
	} else {
		return nil, fmt.Errorf("invalid template name: %s", pinnacleApiModel.TemplateName)
	}
}
