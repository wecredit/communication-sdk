package pinnacleApi

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// HitPinnacleApi calls the Pinnacle GET SMS endpoint using a safely-escaped
// URL and classifies the response into SmsResponse. It avoids logging PII.
func HitPinnacleApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var pinnacleSmsResponse extapimodels.SmsResponse
	pinnacleSmsResponse.IsSent = false

	apiBase := strings.TrimRight(config.Configs.PinnacleSmsApiUrl, "/")
	apiKey := strings.TrimSpace(config.Configs.PinnacleSmsAccessKey)

	if apiBase == "" || apiKey == "" {
		utils.Error(fmt.Errorf("Pinnacle SMS URL or Access Key is missing for client: %s", data.Client))
		pinnacleSmsResponse.ResponseMessage = "Pinnacle SMS URL or Access Key is missing"
		return pinnacleSmsResponse
	}

	// Sender fallback: use Times sender if no explicit Pinnacle sender configured
	sender := strings.TrimSpace(config.Configs.TimesSmsApiSender)
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
	utils.Info(fmt.Sprintf("Pinnacle SMS curl: curl --get '%s'", finalURL))

	// Call API
	apiResponse, err := callPinnacle(finalURL, data)
	if err != nil {
		pinnacleSmsResponse.ResponseMessage = fmt.Sprintf("error calling pinnacle api: %v", err)
		return pinnacleSmsResponse
	}

	// Extract transaction id
	pinnacleSmsResponse.TransactionId = extractTransactionId(apiResponse)

	// Classify
	isSent, respMsg := classifyPinnacleResponse(apiResponse)
	pinnacleSmsResponse.IsSent = isSent
	pinnacleSmsResponse.ResponseMessage = respMsg
	pinnacleSmsResponse.MobileNumber = data.Mobile
	return pinnacleSmsResponse
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

// extractTransactionId pulls common transaction id fields from Pinnacle response.
func extractTransactionId(apiResponse map[string]interface{}) string {
	if apiResponse == nil {
		return ""
	}
	if tx, ok := apiResponse["transactionId"].(string); ok && tx != "" {
		return tx
	}
	if txf, ok := apiResponse["transactionId"].(float64); ok {
		return fmt.Sprintf("%d", int(txf))
	}
	if id, ok := apiResponse["msgid"].(string); ok {
		return id
	}
	if idn, ok := apiResponse["messageId"].(string); ok {
		return idn
	}
	return ""
}

// classifyPinnacleResponse returns (isSent, sanitizedMessage)
func classifyPinnacleResponse(apiResponse map[string]interface{}) (bool, string) {
	if apiResponse == nil {
		return false, "empty response"
	}
	var code int
	if v, ok := apiResponse["ApistatusCode"].(int); ok {
		code = v
	}
	status := fmt.Sprint(apiResponse["status"])
	msg := fmt.Sprint(apiResponse["message"])
	combined := strings.ToLower(status + " " + msg + " " + fmt.Sprint(code))
	isSent := false
	if code >= 200 && code < 300 {
		isSent = true
	} else if strings.Contains(combined, "submitted") || strings.Contains(combined, "success") || strings.Contains(combined, "ok") {
		isSent = true
	}
	sanitized := fmt.Sprintf("status:%s message:%s code:%d", status, msg, code)
	return isSent, sanitized
}
