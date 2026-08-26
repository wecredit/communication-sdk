package pinnacleApi

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	smspolicy "github.com/wecredit/communication-sdk/sdk/policy"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

var pinnacleSensitiveNumber = regexp.MustCompile(`\b[0-9]{10,15}\b`)

const (
	pinnacleMessageTypeTXT = "TXT"
	defaultPinnacleSender  = "WECRPL"
)

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

	sender := strings.TrimSpace(data.TemplateHeader)
	if sender == "" {
		sender = strings.TrimSpace(config.Configs.TimesSmsApiSender)
	}
	if sender == "" {
		sender = defaultPinnacleSender
	}

	message := strings.TrimSpace(data.TemplateText)
	if message == "" {
		message = strings.TrimSpace(data.Description)
	}
	if message == "" {
		pinnacleSmsResponse.ResponseMessage = "Pinnacle SMS message text is empty"
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	entityID := strings.TrimSpace(data.TemplateEntityId)
	if entityID == "" {
		entityID = strings.TrimSpace(config.Configs.PinnacleSmsDltEntityId)
	}

	payload, err := BuildPinnacleJSONPayload(data, sender, message, entityID)
	if err != nil {
		pinnacleSmsResponse.ResponseMessage = err.Error()
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	logPinnacleJSONRequest(data, apiURL, sender, message, entityID, payload)

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

	apiResponse, err := callPinnacleJSON(apiURL, apiKey, payload, data)
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
		// e.g. .../index.php/sms or unexpected suffix → prefer .../sms/json
		idx := strings.LastIndex(lower, "/index.php/sms")
		return configured[:idx] + "/index.php/sms/json"
	default:
		return configured
	}
}

// BuildPinnacleJSONPayload builds the Console SMS JSON body (no secrets).
func BuildPinnacleJSONPayload(data extapimodels.SmsRequestBody, sender, message, entityID string) (map[string]interface{}, error) {
	number, err := NormalizePinnacleMSISDN(data.Mobile)
	if err != nil {
		return nil, err
	}

	msgItem := map[string]interface{}{
		"number": number,
		"text":   message,
	}
	if uid := sanitizePinnacleClientUID(data.CommId); uid != "" {
		msgItem["clientuid"] = uid
	}

	payload := map[string]interface{}{
		"sender":      strings.TrimSpace(sender),
		"message":     []map[string]interface{}{msgItem},
		"messagetype": choosePinnacleMessageType(message),
	}
	if data.DltTemplateId != 0 {
		payload["dlttempid"] = strconv.FormatInt(data.DltTemplateId, 10)
	}
	if strings.TrimSpace(entityID) != "" {
		payload["dltentityid"] = strings.TrimSpace(entityID)
	}
	return payload, nil
}

func choosePinnacleMessageType(message string) string {
	// Console doc also supports CLICK/UCLICK for vendor URL shortening.
	// Default TXT: JSON POST already avoids GET path breakage for https:// links.
	// DLT URL whitelist (5901 URL_NOT_FOUND) remains a registration concern.
	_ = message
	return pinnacleMessageTypeTXT
}

// NormalizePinnacleMSISDN returns digits in 91XXXXXXXXXX form when possible.
func NormalizePinnacleMSISDN(mobile string) (string, error) {
	digits := onlyDigits(mobile)
	switch {
	case len(digits) == 10:
		return "91" + digits, nil
	case len(digits) == 12 && strings.HasPrefix(digits, "91"):
		return digits, nil
	case len(digits) == 11 && strings.HasPrefix(digits, "0"):
		return "91" + digits[1:], nil
	case len(digits) >= 10 && len(digits) <= 15:
		return digits, nil
	default:
		return "", fmt.Errorf("invalid mobile for Pinnacle SMS")
	}
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

func sanitizePinnacleClientUID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func logPinnacleJSONRequest(data extapimodels.SmsRequestBody, apiURL, sender, message, entityID string, payload map[string]interface{}) {
	msgType, _ := payload["messagetype"].(string)
	utils.Info(fmt.Sprintf(
		"Pinnacle SMS JSON request client=%s commId=%s sourceRowId=%d url=%s sender=%s dltTemplateId=%d entityId=%s mobileLen=%d mobileTail=%s messageLen=%d unresolvedVar=%t messagetype=%s",
		data.Client, data.CommId, data.SourceRowId, apiURL, sender, data.DltTemplateId, entityID,
		len(strings.TrimSpace(data.Mobile)), mobileTail(data.Mobile), len(message),
		strings.Contains(message, "{#var#}"), msgType,
	))
}

func mobileTail(mobile string) string {
	digits := onlyDigits(mobile)
	if len(digits) < 4 {
		return "****"
	}
	return digits[len(digits)-4:]
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
	if strings.Contains(combined, "submitted") || strings.Contains(combined, "success") || strings.Contains(combined, "accepted") || strings.Contains(combined, "ok") {
		return outcome.Submitted, sanitized
	}
	if bodyCodeSet && (bodyCode == 429 || bodyCode >= 500) {
		return outcome.FailedRetryable, sanitized
	}
	if bodyCodeSet && bodyCode >= 400 && bodyCode < 500 {
		return outcome.FailedFinal, sanitized
	}
	if !bodyCodeSet && httpCode >= 200 && httpCode < 300 {
		// HTTP 2xx alone is not enough when body uses EC* codes without numeric code.
		if strings.TrimSpace(bodyCodeRaw) == "" && !strings.Contains(combined, "success") && !strings.Contains(combined, "accepted") {
			return outcome.Unknown, sanitized
		}
		if strings.Contains(combined, "success") || strings.Contains(combined, "accepted") {
			return outcome.Submitted, sanitized
		}
	}
	if httpCode == 429 || httpCode >= 500 {
		return outcome.FailedRetryable, sanitized
	}
	if httpCode >= 400 && httpCode < 500 {
		return outcome.FailedFinal, sanitized
	}
	return outcome.Unknown, sanitized
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
