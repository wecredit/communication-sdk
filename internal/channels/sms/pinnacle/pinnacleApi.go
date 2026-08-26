package pinnacleApi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

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

// HitPinnacleApi calls the Pinnacle GET SMS endpoint using a safely-escaped
// URL and classifies the response into SmsResponse. It avoids logging PII.
func HitPinnacleApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var pinnacleSmsResponse extapimodels.SmsResponse
	pinnacleSmsResponse.IsSent = false
	pinnacleSmsResponse.Outcome = outcome.FailedFinal

	apiBase := strings.TrimRight(config.Configs.PinnacleSmsApiUrl, "/")
	apiKey := strings.TrimSpace(config.Configs.PinnacleSmsAccessKey)

	if apiBase == "" || apiKey == "" {
		utils.Error(fmt.Errorf("Pinnacle SMS URL or Access Key is missing for client: %s", data.Client))
		pinnacleSmsResponse.ResponseMessage = "Pinnacle SMS URL or Access Key is missing"
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	// The DLT-approved template header is the sender for this template.
	sender := strings.TrimSpace(data.TemplateHeader)
	if sender == "" {
		sender = strings.TrimSpace(config.Configs.TimesSmsApiSender)
	}
	if sender == "" {
		sender = "WECRPL"
	}

	// Choose message text
	message := strings.TrimSpace(data.TemplateText)
	if message == "" {
		message = strings.TrimSpace(data.Description)
	}

	// Build path: /{sender}/{mobile}/{message}/TXT
	// Use PathEscape to safely include values in the path
	pathParts := []string{
		apiBase,
		url.PathEscape(sender),
		url.PathEscape(data.Mobile),
		url.PathEscape(message),
		"TXT",
	}
	fullPath := strings.Join(pathParts, "/")

	u, err := url.Parse(fullPath)
	if err != nil {
		pinnacleSmsResponse.ResponseMessage = fmt.Sprintf("invalid url: %v", err)
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	// Query params
	q := u.Query()
	if apiKey != "" {
		q.Set("apikey", apiKey)
	}
	if data.DltTemplateId != 0 {
		q.Set("dlttempid", strconv.FormatInt(data.DltTemplateId, 10))
	}
	// include dlt entity id from the template payload or config if present
	entityID := strings.TrimSpace(data.TemplateEntityId)
	if entityID == "" {
		entityID = strings.TrimSpace(config.Configs.PinnacleSmsDltEntityId)
	}
	if entityID != "" {
		q.Set("dltentityid", entityID)
	}
	u.RawQuery = q.Encode()
	finalURL := u.String()

	logPinnacleRequest(data, sender, message, entityID, u)

	// Rate limit the request
	if err := ratelimit.WaitFor(context.Background(), ratelimit.Key(variables.PINNACLE, data.Client)); err != nil {
		pinnacleSmsResponse.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		pinnacleSmsResponse.Outcome = outcome.FailedRetryable
		return pinnacleSmsResponse
	}

	// If the SMS is a regulated marketing SMS and the result is a compliance failure, then we need to evaluate the decision
	if decision := smspolicy.Evaluate(data.Source, data.SourceRowId, data.Channel, data.CampaignDate, smspolicy.Now()); !decision.Allowed() {
		pinnacleSmsResponse.ResponseMessage = decision.ErrorMessage()
		pinnacleSmsResponse.Outcome = outcome.FailedFinal
		return pinnacleSmsResponse
	}

	// Call API only after the final post-rate-limit compliance guard.
	apiResponse, err := callPinnacle(finalURL, data)
	if err != nil {
		utils.Error(fmt.Errorf("Pinnacle SMS API call failed client=%s commId=%s sourceRowId=%d: %v",
			data.Client, data.CommId, data.SourceRowId, err))
		pinnacleSmsResponse.ResponseMessage = fmt.Sprintf("error calling pinnacle api: %v", err)
		pinnacleSmsResponse.Outcome = outcome.ClassifyTransportError(err)
		return pinnacleSmsResponse
	}

	// Extract transaction id
	pinnacleSmsResponse.TransactionId = ExtractTransactionId(apiResponse)

	// Classify
	status, respMsg := ClassifyPinnacleResponse(apiResponse)
	pinnacleSmsResponse.Outcome = status
	pinnacleSmsResponse.IsSent = outcome.IsSentDerived(status)
	pinnacleSmsResponse.ResponseMessage = respMsg
	pinnacleSmsResponse.MobileNumber = data.Mobile

	logPinnacleResponse(data, apiResponse, pinnacleSmsResponse)
	return pinnacleSmsResponse
}

func logPinnacleRequest(data extapimodels.SmsRequestBody, sender, message, entityID string, u *url.URL) {
	mobileLen := len(strings.TrimSpace(data.Mobile))
	mobileTail := mobileTail(data.Mobile)
	msgLen := len(message)
	unresolvedVar := strings.Contains(message, "{#var#}")

	// Redacted curl: never log apikey, full mobile, or SMS body.
	redactedCurl := fmt.Sprintf(
		"curl --location --request GET '%s/%s/[MOBILE_REDACTED len=%d tail=%s]/[MESSAGE_REDACTED len=%d unresolvedVar=%t]/TXT?apikey=[REDACTED]&dltentityid=%s&dlttempid=%d'",
		strings.TrimRight(fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, pathPrefixBeforeSender(u.EscapedPath(), sender)), "/"),
		url.PathEscape(sender),
		mobileLen,
		mobileTail,
		msgLen,
		unresolvedVar,
		entityID,
		data.DltTemplateId,
	)

	utils.Info(fmt.Sprintf(
		"Pinnacle SMS request client=%s commId=%s sourceRowId=%d sender=%s dltTemplateId=%d entityId=%s mobileLen=%d mobileTail=%s messageLen=%d unresolvedVar=%t host=%s curl=%s",
		data.Client, data.CommId, data.SourceRowId, sender, data.DltTemplateId, entityID,
		mobileLen, mobileTail, msgLen, unresolvedVar, u.Host, redactedCurl,
	))
}

func pathPrefixBeforeSender(escapedPath, sender string) string {
	marker := "/" + url.PathEscape(strings.TrimSpace(sender)) + "/"
	idx := strings.Index(escapedPath, marker)
	if idx <= 0 {
		return escapedPath
	}
	return escapedPath[:idx]
}

func mobileTail(mobile string) string {
	digits := strings.TrimSpace(mobile)
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

// BuildPinnacleURL builds the safe Pinnacle URL for GET contract.
func BuildPinnacleURL(base, apiKey, sender, mobile, message string, dltTemplateID int64, dltEntityID string) (string, error) {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return "", fmt.Errorf("empty base url")
	}
	parts := []string{
		base,
		url.PathEscape(strings.TrimSpace(sender)),
		url.PathEscape(strings.TrimSpace(mobile)),
		url.PathEscape(strings.TrimSpace(message)),
		"TXT",
	}
	full := strings.Join(parts, "/")
	u, err := url.Parse(full)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if strings.TrimSpace(apiKey) != "" {
		q.Set("apikey", apiKey)
	}
	if dltTemplateID != 0 {
		q.Set("dlttempid", strconv.FormatInt(dltTemplateID, 10))
	}
	if strings.TrimSpace(dltEntityID) != "" {
		q.Set("dltentityid", dltEntityID)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// callPinnacle executes the GET request and returns parsed JSON map. On error, also send to error queue.
func callPinnacle(urlStr string, data extapimodels.SmsRequestBody) (map[string]interface{}, error) {
	apiResponse, err := utils.ApiHit("GET", urlStr, map[string]string{}, "", "", nil, variables.ContentTypeJSON)
	if err != nil {
		// Push to error queue for further inspection
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
// Prefer provider body code/status when present; HTTP ApistatusCode is fallback only.
// ApiHit always stamps ApistatusCode from the transport layer, which can hide a
// non-success body if HTTP is still 2xx.
func ClassifyPinnacleResponse(apiResponse map[string]interface{}) (string, string) {
	if apiResponse == nil {
		return outcome.Unknown, "empty response"
	}

	bodyCode, bodyCodeSet := pinnacleBodyCode(apiResponse)
	httpCode := pinnacleHTTPCode(apiResponse)
	status := fmt.Sprint(apiResponse["status"])
	msg := fmt.Sprint(apiResponse["message"])
	code := httpCode
	if bodyCodeSet {
		code = bodyCode
	}
	combined := strings.ToLower(status + " " + msg + " " + fmt.Sprint(code))
	sanitized := fmt.Sprintf("status:%s message:%s bodyCode=%d httpCode=%d", status, msg, bodyCode, httpCode)

	if bodyCodeSet && bodyCode >= 200 && bodyCode < 300 {
		return outcome.Submitted, sanitized
	}
	if strings.Contains(combined, "submitted") || strings.Contains(combined, "success") || strings.Contains(combined, "ok") {
		return outcome.Submitted, sanitized
	}
	if bodyCodeSet && (bodyCode == 429 || bodyCode >= 500) {
		return outcome.FailedRetryable, sanitized
	}
	if bodyCodeSet && bodyCode >= 400 && bodyCode < 500 {
		return outcome.FailedFinal, sanitized
	}
	if !bodyCodeSet && httpCode >= 200 && httpCode < 300 {
		return outcome.Submitted, sanitized
	}
	if httpCode == 429 || httpCode >= 500 {
		return outcome.FailedRetryable, sanitized
	}
	if httpCode >= 400 && httpCode < 500 {
		return outcome.FailedFinal, sanitized
	}
	// Ambiguous body without a clear success/failure contract.
	return outcome.Unknown, sanitized
}

func pinnacleBodyCode(apiResponse map[string]interface{}) (int, bool) {
	if v, ok := apiResponse["code"].(int); ok {
		return v, true
	}
	if v, ok := apiResponse["code"].(float64); ok {
		return int(v), true
	}
	return 0, false
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
