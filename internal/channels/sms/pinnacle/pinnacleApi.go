package pinnacleApi

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	pinnaclepayloads "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle/pinnaclePayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	smspolicy "github.com/wecredit/communication-sdk/sdk/policy"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// Redacts 10–15 digit sequences (typically mobiles) from Pinnacle response logs.
var pinnacleSensitiveNumber = regexp.MustCompile(`\b[0-9]{10,15}\b`)

// HitPinnacleApi sends SMS via Pinnacle Console JSON API:
// POST https://api.pinnacle.in/index.php/sms/json
// (avoids GET path encoding issues with https:// links that contain '/').
func HitPinnacleApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var pinnacleSmsResponse extapimodels.SmsResponse
	pinnacleSmsResponse.IsSent = false
	pinnacleSmsResponse.Outcome = outcome.FailedFinal

	apiURL := ResolvePinnacleJSONURL(config.Configs.PinnacleSmsApiUrl)
	apiKey := strings.TrimSpace(config.Configs.PinnacleSmsAccessKey)

	if apiURL == "" || apiKey == "" {
		utils.Error(fmt.Errorf("Pinnacle SMS URL or Access Key is missing for client: %s", data.Client))
		pinnacleSmsResponse.ResponseMessage = "Pinnacle SMS URL or Access Key is missing"
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	apiPayload, err := pinnaclepayloads.GetTemplatePayload(data, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting SMS payload: %v", err))
		pinnacleSmsResponse.ResponseMessage = fmt.Sprintf("error occured in Pinnacle SMS payload: %v for %s", err, data.Client)
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	logPinnacleJSONRequest(data, apiURL, apiPayload)

	if err := ratelimit.WaitFor(context.Background(), ratelimit.Key(variables.PINNACLE, data.Client)); err != nil {
		pinnacleSmsResponse.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		pinnacleSmsResponse.Outcome = outcome.FailedRetryable
		return pinnacleSmsResponse
	}

	if decision := smspolicy.Evaluate(data.Source, data.SourceRowId, data.Channel, data.CampaignDate, smspolicy.Now()); !decision.Allowed() {
		pinnacleSmsResponse.ResponseMessage = decision.ErrorMessage()
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	apiResponse, err := callPinnacleJSON(apiURL, apiKey, apiPayload, data)
	if err != nil {
		utils.Error(fmt.Errorf("Pinnacle SMS API call failed client=%s commId=%s sourceRowId=%d: %v",
			data.Client, data.CommId, data.SourceRowId, err))
		pinnacleSmsResponse.ResponseMessage = fmt.Sprintf("error calling pinnacle api: %v", err)
		pinnacleSmsResponse.Outcome = outcome.ClassifyTransportError(err)
		return pinnacleSmsResponse
	}

	pinnacleSmsResponse.TransactionId = ExtractTransactionId(apiResponse)
	status, respMsg := ClassifyPinnacleResponse(apiResponse)
	pinnacleSmsResponse.Outcome = status
	pinnacleSmsResponse.IsSent = outcome.IsSentDerived(status)
	pinnacleSmsResponse.ResponseMessage = respMsg
	pinnacleSmsResponse.MobileNumber = data.Mobile

	logPinnacleResponse(data, apiResponse, pinnacleSmsResponse)
	return pinnacleSmsResponse
}

// ResolvePinnacleJSONURL normalizes configured send URL to the Console JSON endpoint.
// PINNACLE_SMS_API_URL may still point at /sms/send for older envs.
func ResolvePinnacleJSONURL(configured string) string {
	configured = strings.TrimRight(strings.TrimSpace(configured), "/")
	if configured == "" {
		return ""
	}
	lower := strings.ToLower(configured)
	switch {
	case strings.HasSuffix(lower, "/sms/json"):
		return configured
	case strings.HasSuffix(lower, "/sms/send"):
		return configured[:len(configured)-len("/sms/send")] + "/sms/json"
	case strings.Contains(lower, "/index.php/sms"):
		idx := strings.LastIndex(lower, "/index.php/sms")
		return configured[:idx] + "/index.php/sms/json"
	default:
		return configured
	}
}

func logPinnacleJSONRequest(data extapimodels.SmsRequestBody, apiURL string, payload map[string]interface{}) {
	sender, _ := payload["sender"].(string)
	msgType, _ := payload["messagetype"].(string)
	entityID, _ := payload["dltentityid"].(string)
	messageLen := 0
	unresolvedVar := false
	if msgs, ok := payload["message"].([]map[string]interface{}); ok && len(msgs) > 0 {
		if text, ok := msgs[0]["text"].(string); ok {
			messageLen = len(text)
			unresolvedVar = strings.Contains(text, "{#var#}")
		}
	}
	utils.Info(fmt.Sprintf(
		"Pinnacle SMS JSON request client=%s commId=%s sourceRowId=%d url=%s sender=%s dltTemplateId=%d entityId=%s mobileLen=%d mobileTail=%s messageLen=%d unresolvedVar=%t messagetype=%s",
		data.Client, data.CommId, data.SourceRowId, apiURL, sender, data.DltTemplateId, entityID,
		len(strings.TrimSpace(data.Mobile)), mobileTail(data.Mobile), messageLen,
		unresolvedVar, msgType,
	))
}

func mobileTail(mobile string) string {
	digits := onlyDigits(mobile)
	if len(digits) < 4 {
		return "****"
	}
	return digits[len(digits)-4:]
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func logPinnacleResponse(data extapimodels.SmsRequestBody, apiResponse map[string]interface{}, result extapimodels.SmsResponse) {
	sanitized := sanitizePinnacleResponseForLog(apiResponse)
	raw, err := json.Marshal(sanitized)
	if err != nil {
		raw = []byte(`{"marshalError":true}`)
	}
	utils.Info(fmt.Sprintf(
		"Pinnacle SMS response client=%s commId=%s sourceRowId=%d outcome=%s isSent=%t transactionId=%s body=%s",
		data.Client, data.CommId, data.SourceRowId, result.Outcome, result.IsSent, result.TransactionId, string(raw),
	))
}

func sanitizePinnacleResponseForLog(apiResponse map[string]interface{}) map[string]interface{} {
	if apiResponse == nil {
		return map[string]interface{}{}
	}
	raw, err := json.Marshal(apiResponse)
	if err != nil {
		return map[string]interface{}{"sanitizeError": true}
	}
	redacted := pinnacleSensitiveNumber.ReplaceAllString(string(raw), "[REDACTED]")
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(redacted), &out); err != nil {
		return map[string]interface{}{"raw": redacted}
	}
	return out
}

func callPinnacleJSON(apiURL, apiKey string, payload map[string]interface{}, data extapimodels.SmsRequestBody) (map[string]interface{}, error) {
	headers := map[string]string{
		"apikey":       apiKey,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
	apiResponse, err := utils.ApiHit(variables.PostMethod, apiURL, headers, "", "", payload, variables.ContentTypeJSON)
	if err != nil {
		if qErr := queue.SendMessageWithSubject(queue.SQSClient, data, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); qErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", qErr))
		}
		return nil, err
	}
	return apiResponse, nil
}

// ExtractTransactionId pulls common transaction id fields from Pinnacle response.
// Observed SMS success body: {"code":200,"status":"success","data":[{"uniqueid":"..."}]}.
func ExtractTransactionId(apiResponse map[string]interface{}) string {
	if apiResponse == nil {
		return ""
	}
	if tx, ok := apiResponse["transactionId"].(string); ok && tx != "" {
		return tx
	}
	if txf, ok := apiResponse["transactionId"].(float64); ok {
		return fmt.Sprintf("%d", int(txf))
	}
	if id, ok := apiResponse["msgid"].(string); ok && id != "" {
		return id
	}
	if idn, ok := apiResponse["messageId"].(string); ok && idn != "" {
		return idn
	}
	if id := uniqueIDFromPinnacleData(apiResponse["data"]); id != "" {
		return id
	}
	return ""
}

func uniqueIDFromPinnacleData(data interface{}) string {
	switch v := data.(type) {
	case []interface{}:
		if len(v) == 0 {
			return ""
		}
		return uniqueIDFromPinnacleData(v[0])
	case map[string]interface{}:
		if id, ok := v["uniqueid"].(string); ok {
			return strings.TrimSpace(id)
		}
		if id, ok := v["uniqueId"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// ClassifyPinnacleResponse returns (providerOutcome, sanitizedMessage).
func ClassifyPinnacleResponse(apiResponse map[string]interface{}) (string, string) {
	if apiResponse == nil {
		return outcome.Unknown, "empty response"
	}

	bodyCode, bodyCodeSet, bodyCodeRaw := pinnacleBodyCode(apiResponse)
	httpCode := pinnacleHTTPCode(apiResponse)
	status := fmt.Sprint(apiResponse["status"])
	msg := fmt.Sprint(apiResponse["message"])
	combined := strings.ToLower(status + " " + msg + " " + bodyCodeRaw + " " + fmt.Sprint(bodyCode))
	sanitized := fmt.Sprintf("status:%s message:%s bodyCode=%s httpCode=%d", status, msg, bodyCodeRaw, httpCode)

	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(bodyCodeRaw)), "EC") {
		return outcome.FailedFinal, sanitized
	}
	if bodyCodeSet && bodyCode >= 200 && bodyCode < 300 {
		return outcome.Submitted, sanitized
	}
	// Prefer numeric failure codes over success-looking text (e.g. "not ok" must not win).
	if bodyCodeSet && (bodyCode == 429 || bodyCode >= 500) {
		return outcome.FailedRetryable, sanitized
	}
	if bodyCodeSet && bodyCode >= 400 && bodyCode < 500 {
		return outcome.FailedFinal, sanitized
	}
	if httpCode == 429 || httpCode >= 500 {
		return outcome.FailedRetryable, sanitized
	}
	if httpCode >= 400 && httpCode < 500 {
		return outcome.FailedFinal, sanitized
	}
	if pinnacleLooksSubmitted(status, msg, combined) {
		return outcome.Submitted, sanitized
	}
	return outcome.Unknown, sanitized
}

func pinnacleLooksSubmitted(status, msg, combined string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	m := strings.ToLower(strings.TrimSpace(msg))
	if s == "success" || s == "submitted" || s == "accepted" || s == "ok" {
		return true
	}
	if m == "success" || m == "submitted" || m == "accepted" || m == "ok" {
		return true
	}
	return strings.Contains(combined, "submitted") ||
		strings.Contains(combined, "success") ||
		strings.Contains(combined, "accepted")
}

func pinnacleBodyCode(apiResponse map[string]interface{}) (code int, set bool, raw string) {
	switch v := apiResponse["code"].(type) {
	case int:
		return v, true, strconv.Itoa(v)
	case float64:
		return int(v), true, strconv.Itoa(int(v))
	case string:
		raw = strings.TrimSpace(v)
		if n, err := strconv.Atoi(raw); err == nil {
			return n, true, raw
		}
		return 0, false, raw
	default:
		return 0, false, ""
	}
}

func pinnacleHTTPCode(apiResponse map[string]interface{}) int {
	if v, ok := apiResponse["ApistatusCode"].(int); ok {
		return v
	}
	if v, ok := apiResponse["ApistatusCode"].(float64); ok {
		return int(v)
	}
	return 0
}
