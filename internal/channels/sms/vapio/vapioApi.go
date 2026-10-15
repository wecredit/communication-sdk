package vapioSms

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	vapiopayloads "github.com/wecredit/communication-sdk/internal/channels/sms/vapio/vapioPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	smspolicy "github.com/wecredit/communication-sdk/sdk/policy"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

const defaultVapioSmsAPIURL = "https://vapio.in/api.php"

var vapioSensitiveNumber = regexp.MustCompile(`\b[0-9]{10,15}\b`)

// HitVapioSmsApi sends one WeCredit SMS through Vapio's form API.
func HitVapioSmsApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	return HitVapioSmsApiWithContext(context.Background(), data)
}

func HitVapioSmsApiWithContext(ctx context.Context, data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var vapioSmsResponse extapimodels.SmsResponse
	vapioSmsResponse.IsSent = false
	vapioSmsResponse.Outcome = outcome.FailedFinal

	apiURL := ResolveVapioURL(config.Configs.WeCreditVapioSmsApiUrl)
	if strings.TrimSpace(config.Configs.WeCreditVapioSmsUsername) == "" ||
		strings.TrimSpace(config.Configs.WeCreditVapioSmsApiKey) == "" ||
		strings.TrimSpace(config.Configs.WeCreditVapioSmsRoute) == "" {
		utils.Error(fmt.Errorf("Vapio SMS credentials are incomplete for client: %s", data.Client))
		vapioSmsResponse.ResponseMessage = "Vapio SMS credentials are incomplete"
		vapioSmsResponse.Outcome = outcome.FailedFinal
		return vapioSmsResponse
	}

	apiPayload, err := vapiopayloads.GetTemplatePayload(data, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting Vapio SMS payload: %v", err))
		vapioSmsResponse.ResponseMessage = fmt.Sprintf("error occured in Vapio SMS payload: %v for %s", err, data.Client)
		vapioSmsResponse.Outcome = outcome.FailedFinal
		return vapioSmsResponse
	}

	logVapioRequest(data, apiURL, apiPayload)

	if decision := smspolicy.Evaluate(data.Source, data.SourceRowId, data.Channel, data.CampaignDate, smspolicy.Now()); !decision.Allowed() {
		vapioSmsResponse.ResponseMessage = decision.ErrorMessage()
		vapioSmsResponse.Outcome = outcome.FailedFinal
		return vapioSmsResponse
	}

	if err := ratelimit.WaitFor(ctx, ratelimit.KeyWithChannel(variables.VAPIO, data.Client, "sms")); err != nil {
		vapioSmsResponse.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		vapioSmsResponse.Outcome = outcome.FailedRetryable
		return vapioSmsResponse
	}

	// Username and API key travel in the form body. Leave basic auth empty.
	apiResponse, err := utils.ApiHit(variables.PostMethod, apiURL, nil, "", "", apiPayload, variables.ContentTypeFormEncoded)
	if err != nil {
		utils.Error(fmt.Errorf("Vapio SMS API call failed client=%s commId=%s sourceRowId=%d: %v",
			data.Client, data.CommId, data.SourceRowId, err))
		vapioSmsResponse.ResponseMessage = fmt.Sprintf("error calling vapio api: %v", err)
		vapioSmsResponse.Outcome = outcome.ClassifyTransportError(err)
		return vapioSmsResponse
	}

	status, respMsg, txnID := ClassifyVapioResponse(apiResponse)
	vapioSmsResponse.TransactionId = txnID
	vapioSmsResponse.Outcome = status
	vapioSmsResponse.IsSent = outcome.IsSentDerived(status)
	vapioSmsResponse.ResponseMessage = respMsg
	vapioSmsResponse.MobileNumber = data.Mobile

	logVapioResponse(data, apiResponse, vapioSmsResponse)
	return vapioSmsResponse
}

// ResolveVapioURL uses the configured endpoint, or the public Vapio URL when unset.
func ResolveVapioURL(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return defaultVapioSmsAPIURL
	}
	return configured
}

// ClassifyVapioResponse maps the Vapio JSON body to an SMS outcome.
// Success is status OK plus a non-empty msg-id.
func ClassifyVapioResponse(apiResponse map[string]interface{}) (string, string, string) {
	if apiResponse == nil {
		return outcome.Unknown, "empty response", ""
	}

	httpCode := utils.HTTPStatusFromResponse(apiResponse)
	status := stringField(apiResponse, "status")
	message := stringField(apiResponse, "message")
	msgID := stringField(apiResponse, "msg-id")
	respMsg := message
	if respMsg == "" {
		respMsg = status
	}

	if httpCode == 429 || httpCode >= 500 {
		return outcome.FailedRetryable, respMsg, ""
	}

	if httpCode != 0 && (httpCode < 200 || httpCode >= 300) {
		return outcome.FailedFinal, respMsg, ""
	}

	if strings.EqualFold(status, "OK") && msgID != "" {
		if message == "" {
			message = "Message Submitted successfully"
		}

		return outcome.Submitted, message, msgID
	}

	if status == "" && msgID == "" {
		return outcome.Unknown, respMsg, ""
	}

	if respMsg == "" {
		respMsg = "Vapio SMS was not accepted"
	}

	return outcome.FailedFinal, respMsg, ""
}

func logVapioRequest(data extapimodels.SmsRequestBody, apiURL string, payload map[string]string) {
	text := payload["text"]
	utils.Info(fmt.Sprintf(
		"Vapio SMS request client=%s commId=%s sourceRowId=%d url=%s sender=%s route=%s dltTemplateId=%s mobileLen=%d mobileTail=%s messageLen=%d unresolvedVar=%t",
		data.Client, data.CommId, data.SourceRowId, apiURL, payload["senderid"], payload["route"], payload["TID"],
		len(payload["mobile"]), mobileTail(payload["mobile"]), len(text), strings.Contains(text, "{#var#}"),
	))
}

func logVapioResponse(data extapimodels.SmsRequestBody, apiResponse map[string]interface{}, result extapimodels.SmsResponse) {
	sanitized := sanitizeVapioResponseForLog(apiResponse)
	raw, err := json.Marshal(sanitized)
	if err != nil {
		raw = []byte(`{"marshalError":true}`)
	}

	utils.Info(fmt.Sprintf(
		"Vapio SMS response client=%s commId=%s sourceRowId=%d outcome=%s isSent=%t transactionId=%s body=%s",
		data.Client, data.CommId, data.SourceRowId, result.Outcome, result.IsSent, result.TransactionId, string(raw),
	))
}

func sanitizeVapioResponseForLog(apiResponse map[string]interface{}) map[string]interface{} {
	if apiResponse == nil {
		return map[string]interface{}{}
	}

	raw, err := json.Marshal(apiResponse)
	if err != nil {
		return map[string]interface{}{"sanitizeError": true}
	}

	redacted := vapioSensitiveNumber.ReplaceAllString(string(raw), "[REDACTED]")
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(redacted), &out); err != nil {
		return map[string]interface{}{"raw": redacted}
	}

	delete(out, "apikey")
	delete(out, "username")
	return out
}

func mobileTail(mobile string) string {
	digits := onlyDigits(mobile)
	if len(digits) < 4 {
		return "****"
	}

	return digits[len(digits)-4:]
}

func onlyDigits(mobile string) string {
	var b strings.Builder
	for _, r := range mobile {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}

	return b.String()
}

func stringField(apiResponse map[string]interface{}, key string) string {
	raw, ok := apiResponse[key]
	if !ok || raw == nil {
		return ""
	}

	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		text := strings.TrimSpace(fmt.Sprint(v))
		if text == "<nil>" {
			return ""
		}
		return text
	}
}
