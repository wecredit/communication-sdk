package pinnacleWhatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	pinnaclepayloads "github.com/wecredit/communication-sdk/internal/channels/whatsapp/pinnacle/pinnaclePayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// pinnacleAPIResponse is the Pinnacle WA send / error body contract.
type pinnacleAPIResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    *struct {
		Details string `json:"details"`
	} `json:"data"`
	Error *struct {
		Message   string `json:"message"`
		ErrorData *struct {
			Details string `json:"details"`
		} `json:"error_data"`
	} `json:"error"`
}

func HitPinnacleWhatsappApi(pinnacleApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	apiUrl, apiKey := resolvePinnacleMessageEndpoint(pinnacleApiModel.Client, pinnacleApiModel.AppId)
	if apiUrl == "" || apiKey == "" {
		utils.Error(fmt.Errorf("pinnacle WA message URL or API key is not set"))
		responseBody.ResponseMessage = "pinnacle WA message URL or API key is not set"
		return responseBody
	}

	apiHeader := map[string]string{
		"apikey":       apiKey,
		"Content-Type": "application/json",
	}

	apiPayload, err := GetPinnaclePayload(pinnacleApiModel)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	if err := ratelimit.WaitFor(context.Background(), ratelimit.KeyWithChannel(variables.PINNACLE, pinnacleApiModel.Client, "whatsapp")); err != nil {
		responseBody.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		return responseBody
	}

	fmt.Println("Pinnacle Whatsapp payload:", apiPayload)

	jsonBytes, _ := json.Marshal(apiPayload)
	responseBody.RawPayload = string(jsonBytes)
	utils.Debug(fmt.Sprintf("Pinnacle Whatsapp payload for mobile: %s and templateName: %s is: %s", pinnacleApiModel.Mobile, pinnacleApiModel.TemplateName, string(jsonBytes)))

	var apiResponse pinnacleAPIResponse
	statusCode, rawBody, err := utils.ApiHitJSON("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON, &apiResponse)
	if rawBody != "" {
		responseBody.RawResponse = rawBody
	}
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Pinnacle Wp API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, pinnacleApiModel, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Pinnacle Wp API: %v", err)
		return responseBody
	}

	if statusCode == http.StatusOK {
		responseBody.IsSent = true
		responseBody.ResponseMessage = "Message submitted successfully"
		if len(apiResponse.Messages) > 0 {
			responseBody.TransactionId = strings.TrimSpace(apiResponse.Messages[0].ID)
		}
	} else {
		responseBody.IsSent = false
		responseBody.ResponseMessage = pinnacleWhatsappErrorBodyMessage(apiResponse)
	}

	fmt.Println("PINNACLE FINAL WHATSAPP RESPONSE:", responseBody)
	// lead_id is our CommId — we set it on the outbound payload.
	if leadID := strings.TrimSpace(pinnacleApiModel.CommId); leadID != "" {
		responseBody.ResponseMessage = responseBody.ResponseMessage + " | " + leadID
	}
	return responseBody
}

// resolvePinnacleMessageEndpoint picks URL + API key.
// WeCredit marketing: prefer {PINNACLE_WP_BASE_URL|list base}/{AppId}/messages (hermis parity).
// Full PINNACLE_WP_MESSAGE_API_URL is the override when base+AppId cannot be built.
// ZapCash keeps its dedicated full URL + key.
func resolvePinnacleMessageEndpoint(client, appID string) (apiURL, apiKey string) {
	if client == variables.ZapCash {
		return strings.TrimSpace(config.Configs.PinnacleZapcashWhatsappMessageApiUrl),
			strings.TrimSpace(config.Configs.PinnacleZapcashWhatsappApiKey)
	}

	apiKey = strings.TrimSpace(config.Configs.PinnacleWhatsappApiKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(config.Configs.PinnacleZapcashWhatsappApiKey)
	}

	appID = strings.TrimSpace(appID)
	base := strings.TrimSpace(config.Configs.PinnacleWhatsappBaseUrl)
	if base == "" {
		base = strings.TrimSpace(config.Configs.PinnacleWhatsappTemplateListBaseUrl)
	}

	if appID != "" && base != "" {
		return strings.TrimRight(base, "/") + "/" + strings.Trim(appID, "/") + "/messages", apiKey
	}

	apiURL = strings.TrimSpace(config.Configs.PinnacleWhatsappMessageApiUrl)
	if apiURL == "" {
		apiURL = strings.TrimSpace(config.Configs.PinnacleZapcashWhatsappMessageApiUrl)
	}

	return apiURL, apiKey
}

// ResolvePinnacleMessageURL is exported for unit tests.
func ResolvePinnacleMessageURL(client, appID string) string {
	u, _ := resolvePinnacleMessageEndpoint(client, appID)
	return u
}

func pinnacleWhatsappErrorBodyMessage(apiResponse pinnacleAPIResponse) string {
	if apiResponse.Error != nil {
		details := ""
		if apiResponse.Error.ErrorData != nil {
			details = apiResponse.Error.ErrorData.Details
		}
		return formatPinnacleMessageAndDetails(apiResponse.Error.Message, details)
	}
	if apiResponse.Status == "failed" {
		details := ""
		if apiResponse.Data != nil {
			details = apiResponse.Data.Details
		}
		return formatPinnacleMessageAndDetails(apiResponse.Message, details)
	}
	return "failed to send message"
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

// GetPinnaclePayload mirrors Hermis: "utility" in Process → utility payload; otherwise media
// (marketing / image-header templates). TemplateName substring matching is intentionally not used.
// Exported for unit tests under test/pinnacleWhatsapp.
func GetPinnaclePayload(pinnacleApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	if strings.Contains(strings.ToLower(pinnacleApiModel.Process), "utility") {
		return pinnaclepayloads.GetPinnacleUtilityPayload(pinnacleApiModel), nil
	}
	return pinnaclepayloads.GetPinnacleMediaPayload(pinnacleApiModel), nil
}
