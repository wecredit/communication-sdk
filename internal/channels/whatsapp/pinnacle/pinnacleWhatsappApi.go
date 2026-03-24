package pinnacleWhatsapp

import (
	"encoding/json"
	"fmt"
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

	// fmt.Println("WHatsapp payload:", apiPayload)

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

	fmt.Println("PINNACLE FINAL WHATSAPP RESPONSE:", responseBody)

	return responseBody
}

func getPayload(pinnacleApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(pinnacleApiModel.TemplateName, "utility") {
		// For Utility Payload
		fmt.Println("Generating Utility Payload for Pinnacle WhatsApp API")
		return pinnaclepayloads.GetPinnacleUtilityPayload(pinnacleApiModel), nil
	} else {
		return pinnaclepayloads.GetPinnacleMediaPayload(pinnacleApiModel), nil
	}
}
