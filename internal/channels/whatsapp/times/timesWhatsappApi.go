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

// timesAPIResponse is the Times WA sync send contract (known fields only).
type timesAPIResponse struct {
	Status    bool            `json:"status"`
	Message   string          `json:"message"`
	MessageID string          `json:"message_id"`
	ResJSON   json.RawMessage `json:"res_json"`
}

// timesAsyncAPIResponse is the Times WA async accept contract.
type timesAsyncAPIResponse struct {
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	Code      json.RawMessage `json:"code"`
	CtTraceID string          `json:"ct_trace_id"`
}

type timesResMessages struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

func HitTimesWhatsappApi(timesApiModel extapimodels.WhatsappRequestBody) extapimodels.WhatsappResponse {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

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

	mode, err := config.ResolveTimesWpMode(config.Configs.TimesWpApiMode)
	if err != nil {
		responseBody.ResponseMessage = err.Error()
		return responseBody
	}
	apiURL, err := config.TimesWpSendURL(mode, config.Configs.TimesWpApiUrl, config.Configs.TimesWpAsyncApiUrl)
	if err != nil {
		responseBody.ResponseMessage = err.Error()
		return responseBody
	}

	if err := ratelimit.WaitFor(context.Background(), ratelimit.KeyWithChannel(variables.TIMES, timesApiModel.Client, config.TimesWpRateChannel(mode))); err != nil {
		responseBody.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		return responseBody
	}

	if raw, mErr := json.Marshal(apiPayload); mErr == nil {
		responseBody.RawPayload = string(raw)
	}

	if mode == config.TimesWpModeAsync {
		return hitTimesAsync(timesApiModel, apiURL, apiHeader, apiPayload, responseBody)
	}
	return hitTimesSync(timesApiModel, apiURL, apiHeader, apiPayload, responseBody)
}

func hitTimesSync(timesApiModel extapimodels.WhatsappRequestBody, apiURL string, apiHeader map[string]string, apiPayload map[string]interface{}, responseBody extapimodels.WhatsappResponse) extapimodels.WhatsappResponse {
	var apiResponse timesAPIResponse
	_, rawBody, err := utils.ApiHitJSON("POST", apiURL, apiHeader, "", "", apiPayload, variables.ContentTypeJSON, &apiResponse)
	if rawBody != "" {
		responseBody.RawResponse = rawBody
	}
	if err != nil {
		return timesTransportFailure(timesApiModel, responseBody, err)
	}

	parsed := applyTimesSyncResponse(apiResponse)
	parsed.RawPayload = responseBody.RawPayload
	parsed.RawResponse = responseBody.RawResponse
	return parsed
}

func hitTimesAsync(timesApiModel extapimodels.WhatsappRequestBody, apiURL string, apiHeader map[string]string, apiPayload map[string]interface{}, responseBody extapimodels.WhatsappResponse) extapimodels.WhatsappResponse {
	statusCode, rawBody, err := utils.ApiHitJSON("POST", apiURL, apiHeader, "", "", apiPayload, variables.ContentTypeJSON, nil)
	if rawBody != "" {
		responseBody.RawResponse = rawBody
	}
	if err != nil {
		return timesTransportFailure(timesApiModel, responseBody, err)
	}

	parsed, classifyErr := ClassifyTimesAsyncSend(statusCode, []byte(rawBody))
	parsed.RawPayload = responseBody.RawPayload
	parsed.RawResponse = responseBody.RawResponse
	if classifyErr != nil {
		return timesTransportFailure(timesApiModel, parsed, classifyErr)
	}
	return parsed
}

func timesTransportFailure(timesApiModel extapimodels.WhatsappRequestBody, responseBody extapimodels.WhatsappResponse, hitErr error) extapimodels.WhatsappResponse {
	utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", hitErr))
	if queueErr := queue.SendMessageWithSubject(queue.SQSClient, timesApiModel, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, hitErr.Error()); queueErr != nil {
		utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
	}
	responseBody.IsSent = false
	if strings.TrimSpace(responseBody.ResponseMessage) == "" {
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Times Wp API: %v", hitErr)
	}
	return responseBody
}

// ApplyTimesSyncBody unmarshals a sync Times body. status must be a JSON bool.
func ApplyTimesSyncBody(raw []byte) (extapimodels.WhatsappResponse, error) {
	var apiResponse timesAPIResponse
	if err := json.Unmarshal(raw, &apiResponse); err != nil {
		return extapimodels.WhatsappResponse{}, fmt.Errorf("error unmarshalling response: %v", err)
	}
	return applyTimesSyncResponse(apiResponse), nil
}

func applyTimesSyncResponse(apiResponse timesAPIResponse) extapimodels.WhatsappResponse {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false
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

// ClassifyTimesAsyncSend reads an async accept body.
// A non-nil error is a transport failure (non-JSON or HTTP 5xx) and must use the error queue.
// HTTP 200 failure and HTTP 403 are terminal vendor rejects.
func ClassifyTimesAsyncSend(statusCode int, raw []byte) (extapimodels.WhatsappResponse, error) {
	var responseBody extapimodels.WhatsappResponse
	responseBody.IsSent = false

	var body timesAsyncAPIResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return responseBody, fmt.Errorf("error unmarshalling response: %v", err)
	}

	if statusCode >= 500 {
		msg := strings.TrimSpace(body.Message)
		if msg == "" {
			msg = "internal service error"
		}
		return responseBody, fmt.Errorf("Times WhatsApp async HTTP %d: %s", statusCode, msg)
	}

	status := strings.ToLower(strings.TrimSpace(body.Status))
	if status == "success" && (statusCode == 200 || statusCode == 0) {
		traceID := strings.TrimSpace(body.CtTraceID)
		if traceID == "" {
			responseBody.ResponseMessage = "accepted without ct_trace_id"
			return responseBody, nil
		}
		responseBody.IsSent = true
		responseBody.TransactionId = traceID
		label := strings.TrimSpace(body.Message)
		if label == "" {
			label = "accepted"
		}
		responseBody.ResponseMessage = fmt.Sprintf("%s | ct_trace_id: %s", label, traceID)
		return responseBody, nil
	}

	if status == "failure" || statusCode == httpStatusUnauthorized || statusCode == httpStatusBadRequest {
		responseBody.ResponseMessage = asyncFailureMessage(body)
		return responseBody, nil
	}

	return responseBody, fmt.Errorf("Times WhatsApp async HTTP %d", statusCode)
}

const (
	httpStatusBadRequest   = 400
	httpStatusUnauthorized = 403
)

func asyncFailureMessage(body timesAsyncAPIResponse) string {
	code := asyncCode(body.Code)
	msg := strings.TrimSpace(body.Message)
	switch {
	case msg != "" && code != "":
		return fmt.Sprintf("Message: %s | Code: %s", msg, code)
	case msg != "":
		return fmt.Sprintf("Message: %s", msg)
	case code != "":
		return fmt.Sprintf("Code: %s", code)
	default:
		return "Times WhatsApp async request failed"
	}
}

func asyncCode(raw json.RawMessage) string {
	code := strings.TrimSpace(string(raw))
	if code == "" || code == "null" {
		return ""
	}
	return strings.Trim(code, `"`)
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
