package timesWhatsapp

import (
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	timespayloads "github.com/wecredit/communication-sdk/internal/channels/whatsapp/times/timesPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitTimesWhatsappApi(timesApiModel extapimodels.WhatsappRequestBody) (extapimodels.WhatsappResponse, error) {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false
	// Getting the API URL
	apiUrl := config.Configs.TimesWpApiUrl

	// Getting the WhatsApp Authorization token
	apiAuthorization := config.Configs.TimesWpAPIToken

	// Setting the API header
	apiHeader := map[string]string{
		"Authorization": apiAuthorization,
		"Content-Type":  "application/json",
	}

	// Get api payload
	apiPayload, err := getPayload(timesApiModel)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
		responseBody.ResponseMessage = fmt.Sprintf("error occured while getting Times Whatsapp payload: %v", err)
		return responseBody, nil
	}

	fmt.Println("Times Whatsapp payload:", apiPayload)

	apiResponse, err := utils.ApiHit("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Times Wp API: %v", err)
		return responseBody, err
	}

	fmt.Println("ApiResponse Times:", apiResponse)
	status := apiResponse["status"].(bool)
	if status {
		responseBody.IsSent = true
		// Extract `message_id`
		var messageID string
		if apiResponse["message_id"] != nil {
			messageID, _ = apiResponse["message_id"].(string)
		} else {
			messageID = "null"
		}

		// Extract `messages[0].id` from `res_json`
		var messageWamID string
		if resJson, ok := apiResponse["res_json"].(map[string]interface{}); ok {
			if messages, ok := resJson["messages"].([]interface{}); ok && len(messages) > 0 {
				if firstMessage, ok := messages[0].(map[string]interface{}); ok {
					messageWamID, _ = firstMessage["id"].(string)
				}
			}
		}

		// Build the final response message
		parts := []string{
			fmt.Sprintf("MessageID: %s", messageID),
		}

		if messageWamID != "" {
			parts = append(parts, fmt.Sprintf("WAMID: %s", messageWamID))
		}

		responseBody.TransactionId = messageWamID
		responseBody.ResponseMessage = strings.Join(parts, " | ")
	} else { // Handle error case
		// Extract message
		message, _ := apiResponse["message"].(string)

		// Extract message_id (handle nil case)
		var messageID string
		if apiResponse["message_id"] != nil {
			messageID, _ = apiResponse["message_id"].(string)
		} else {
			messageID = "null"
		}

		// Extract res_json errors
		var errorMsgs []string
		if resJSON, ok := apiResponse["res_json"].([]interface{}); ok {
			for _, item := range resJSON {
				if m, ok := item.(map[string]interface{}); ok {
					for field, msg := range m {
						errorMsgs = append(errorMsgs, fmt.Sprintf("%s: %s", field, msg))
					}
				}
			}
		}

		if len(errorMsgs) > 0 {
			responseBody.ResponseMessage = fmt.Sprintf("Message: %s | MessageID: %s | Errors: %s",
				message,
				messageID,
				strings.Join(errorMsgs, " | "),
			)
		} else {
			responseBody.ResponseMessage = fmt.Sprintf("Message: %s | MessageID: %s", message, messageID)
		}

	}

	return responseBody, nil
}

func getPayload(timesApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(timesApiModel.Process, "utility") {
		// For Utility Payload
		return timespayloads.GetTimesUtilityPayload(timesApiModel)
	} else {
		return timespayloads.GetTimesMediaPayload(timesApiModel)
	}

}
