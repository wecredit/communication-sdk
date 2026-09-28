package timesWhatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	timespayloads "github.com/wecredit/communication-sdk/internal/channels/whatsapp/times/timesPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// timesAPIResponse is the Times WA send contract (known fields only).
type timesAPIResponse struct {
	Status    bool            `json:"status"`
	Message   string          `json:"message"`
	MessageID string          `json:"message_id"`
	ResJSON   json.RawMessage `json:"res_json"`
}

type timesResMessages struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

func HitTimesWhatsappApi(timesApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false
	apiUrl := config.Configs.TimesWpApiUrl

	apiAuthorization := strings.TrimSpace(timesApiModel.AppId)
	if apiAuthorization == "" {
		apiAuthorization = config.Configs.TimesWpAPIToken
	}

	apiHeader := map[string]string{
		"Authorization": apiAuthorization,
		"Content-Type":  "application/json",
	}

	apiPayload, err := getPayload(timesApiModel)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
		responseBody.ResponseMessage = fmt.Sprintf("error occured while getting Times Whatsapp payload: %v", err)
		return responseBody
	}

	if err := ratelimit.WaitFor(context.Background(), ratelimit.KeyWithChannel(variables.TIMES, timesApiModel.Client, "whatsapp")); err != nil {
		responseBody.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		return responseBody
	}

	if raw, mErr := json.Marshal(apiPayload); mErr == nil {
		responseBody.RawPayload = string(raw)
	}

	fmt.Println("Times Whatsapp payload:", apiPayload)

	var apiResponse timesAPIResponse
	_, rawBody, err := utils.ApiHitJSON("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON, &apiResponse)
	if rawBody != "" {
		responseBody.RawResponse = rawBody
	}
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, timesApiModel, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Times Wp API: %v", err)
		return responseBody
	}

	fmt.Println("ApiResponse Times:", rawBody)
	if apiResponse.Status {
		responseBody.IsSent = true
		messageID := strings.TrimSpace(apiResponse.MessageID)
		if messageID == "" {
			messageID = "null"
		}

		messageWamID := timesWamid(apiResponse.ResJSON)
		parts := []string{fmt.Sprintf("MessageID: %s", messageID)}
		if messageWamID != "" {
			parts = append(parts, fmt.Sprintf("WAMID: %s", messageWamID))
		}
		responseBody.TransactionId = messageWamID
		responseBody.ResponseMessage = strings.Join(parts, " | ")
		return responseBody
	}

	messageID := strings.TrimSpace(apiResponse.MessageID)
	if messageID == "" {
		messageID = "null"
	}
	errorMsgs := timesErrorFields(apiResponse.ResJSON)
	if len(errorMsgs) > 0 {
		responseBody.ResponseMessage = fmt.Sprintf("Message: %s | MessageID: %s | Errors: %s",
			apiResponse.Message, messageID, strings.Join(errorMsgs, " | "))
	} else {
		responseBody.ResponseMessage = fmt.Sprintf("Message: %s | MessageID: %s", apiResponse.Message, messageID)
	}
	return responseBody
}

func timesWamid(resJSON json.RawMessage) string {
	if len(resJSON) == 0 {
		return ""
	}
	var parsed timesResMessages
	if err := json.Unmarshal(resJSON, &parsed); err != nil {
		return ""
	}
	if len(parsed.Messages) == 0 {
		return ""
	}
	return strings.TrimSpace(parsed.Messages[0].ID)
}

func timesErrorFields(resJSON json.RawMessage) []string {
	if len(resJSON) == 0 {
		return nil
	}
	var items []map[string]string
	if err := json.Unmarshal(resJSON, &items); err != nil {
		return nil
	}
	var errorMsgs []string
	for _, m := range items {
		for field, msg := range m {
			errorMsgs = append(errorMsgs, fmt.Sprintf("%s: %s", field, msg))
		}
	}
	return errorMsgs
}

// getPayload mirrors Hermis: "utility" in Process → utility payload; otherwise media.
func getPayload(timesApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(strings.ToLower(timesApiModel.Process), "utility") {
		return timespayloads.GetTimesUtilityPayload(timesApiModel)
	}
	return timespayloads.GetTimesMediaPayload(timesApiModel)
}
