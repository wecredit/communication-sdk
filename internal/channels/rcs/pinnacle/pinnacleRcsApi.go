package pinnacleRcs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

const pinnacleRcsCorrelationIDLen = 20

func HitPinnacleRcsApi(data extapimodels.RcsRequestBody) extapimodels.RcsResponse {
	var responseBody extapimodels.RcsResponse
	responseBody.IsSent = false
	var apiURL, apiKey, botID string
	if data.Client == variables.ZapCash {
		apiURL = config.Configs.PinnacleZapcashRcsApiUrl
		apiKey = config.Configs.PinnacleZapcashRcsApiKey
		if data.TemplateCategory == variables.TransactionalTemplateCategory {
			botID = config.Configs.PinnacleZapcashRcsTransactionalBotId
		} else {
			botID = config.Configs.PinnacleZapcashRcsPromotionalBotId
		}
	}
	if apiURL == "" || apiKey == "" || botID == "" {
		utils.Error(fmt.Errorf("Pinnacle RCS URL, API key, or bot id is not set for client %s", data.Client))
		responseBody.ResponseMessage = "Pinnacle RCS URL, API key, or bot id is not set"
		return responseBody
	}
	ttl := strings.TrimSpace(config.Configs.PinnacleZapcashRcsTtl)
	if ttl == "" {
		ttl = "10"
	}
	apiPayload := buildPinnacleRcsSendPayload(data, ttl)
	jsonBytes, _ := json.Marshal(apiPayload)
	utils.Debug(fmt.Sprintf("Pinnacle RCS payload for mobile: %s templateId: %s: %s", data.Mobile, data.TemplateName, string(jsonBytes)))
	apiHeader := map[string]string{
		"Content-Type": "application/json",
		"apikey":       apiKey,
		"botid":        botID,
	}
	apiResponse, err := utils.ApiHit(variables.PostMethod, apiURL, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Pinnacle RCS API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, data, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		responseBody.ResponseMessage = fmt.Sprintf("error occured while hitting into Pinnacle RCS API: %v", err)
		return responseBody
	}
	statusCode := apiResponse["ApistatusCode"].(int)
	if statusCode == http.StatusOK {
		responseBody.IsSent = true
		responseBody.ResponseMessage = "Message submitted successfully"
		if msgID, ok := pinnacleRcsMessageID(apiResponse); ok {
			responseBody.TransactionId = msgID
		}
		return responseBody
	}
	responseBody.IsSent = false
	responseBody.ResponseMessage = pinnacleRcsErrorBodyMessage(apiResponse)
	return responseBody
}

func buildPinnacleRcsSendPayload(data extapimodels.RcsRequestBody, ttl string) map[string]interface{} {
	rcsKeys := splitCommaVariableKeys(data.TemplateVariables)
	smsKeys := splitCommaVariableKeys(data.SmsFallbackVariables)

	msg := map[string]interface{}{
		"to":                    formatPinnacleRcsTo(data.Mobile),
		"templateId":            data.TemplateName,
		"ttl":                   ttl,
		"isSMSFallbackRequired": true,
		"variables":             buildPinnacleRcsVariablePairs(rcsKeys, data),
		"smsVariables":          buildPinnacleRcsVariablePairs(smsKeys, data),
	}
	return map[string]interface{}{
		"category":      pinnacleRcsCategory(data.TemplateCategory),
		"correlationId": pinnacleRcsCorrelationID(),
		"messages":      []map[string]interface{}{msg},
	}
}

func pinnacleRcsCorrelationID() string {
	b := make([]byte, pinnacleRcsCorrelationIDLen/2)
	if _, err := rand.Read(b); err != nil {
		utils.Error(fmt.Errorf("failed to generate pinnacle RCS correlation id: %v", err))
		return fmt.Sprintf("%0*d", pinnacleRcsCorrelationIDLen, time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func pinnacleRcsCategory(templateCategory string) string {
	switch templateCategory {
	case variables.TransactionalTemplateCategory:
		return "transactional"
	case variables.PromotionalTemplateCategory:
		return "promotional"
	default:
		return "promotional"
	}
}

func formatPinnacleRcsTo(msisdn string) string {
	s := strings.TrimSpace(msisdn)
	if strings.HasPrefix(s, "+") {
		return s
	}
	s = strings.TrimPrefix(s, "91")
	s = strings.TrimPrefix(s, "0")
	if len(s) == 10 {
		return "+91" + s
	}
	return "+91" + s
}

func splitCommaVariableKeys(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	var keys []string
	for _, raw := range parts {
		k := strings.TrimSpace(raw)
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

func buildPinnacleRcsVariablePairs(keys []string, data extapimodels.RcsRequestBody) []map[string]interface{} {
	var out []map[string]interface{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		val := resolvePinnacleRcsVariableValue(key, data)
		if val == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"key":   key,
			"value": val,
		})
	}
	return out
}

func resolvePinnacleRcsVariableValue(key string, data extapimodels.RcsRequestBody) string {
	switch key {
	case "CustomerName", "Customer_Name":
		if data.CustomerName != "" {
			return data.CustomerName
		}
		return "Customer"
	case "DueDate":
		dueDateStr := data.DueDate
		formatted := dueDateStr
		layouts := []string{
			time.RFC3339,
			"2006-01-02 15:04:05 -0700 MST",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, dueDateStr); err == nil {
				formatted = t.Format("2006-01-02")
				break
			}
		}
		if formatted == "" {
			utils.Error(fmt.Errorf("invalid DueDate format: %s", dueDateStr))
		}
		return formatted
	case "LoanId":
		return data.LoanId
	case "ApplicationNumber":
		return data.ApplicationNumber
	case "EmiAmount":
		return data.EmiAmount
	case "TotalPayableAmount":
		return data.TotalPayableAmount
	case "TodayPayableAmount":
		return data.TodayPayableAmount
	case "SavingAmount":
		return data.SavingAmount
	case "BounceCharge":
		return data.BounceCharge
	case "ZapcashApp":
		return "ZapCash App"
	case "Description":
		return data.Description
	case "Link":
		return data.PaymentLink
	default:
		return ""
	}
}

func pinnacleRcsMessageID(apiResponse map[string]interface{}) (string, bool) {
	if data, ok := apiResponse["data"].([]interface{}); ok && len(data) > 0 {
		if id, ok := pinnacleRcsIDFromMessage(data[0]); ok {
			return id, true
		}
	}
	if data, ok := apiResponse["data"].(map[string]interface{}); ok {
		if id, ok := pinnacleRcsIDFromMessage(data); ok {
			return id, true
		}
	}
	for _, key := range []string{"uniqueId", "messageId", "id", "requestId"} {
		if id := stringField(apiResponse[key]); id != "" {
			return id, true
		}
	}
	if msgs, ok := apiResponse["messages"].([]interface{}); ok && len(msgs) > 0 {
		if id, ok := pinnacleRcsIDFromMessage(msgs[0]); ok {
			return id, true
		}
	}
	return "", false
}

func pinnacleRcsIDFromMessage(v interface{}) (string, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return "", false
	}
	for _, key := range []string{"uniqueId", "messageId", "id"} {
		if id := stringField(m[key]); id != "" {
			return id, true
		}
	}
	return "", false
}

func pinnacleRcsErrorBodyMessage(apiResponse map[string]interface{}) string {
	if code := stringField(apiResponse["code"]); code != "" {
		return formatPinnacleRcsMessageAndDetails(code, stringField(apiResponse["status"]))
	}
	if errObj, ok := apiResponse["error"].(map[string]interface{}); ok {
		return formatPinnacleRcsMessageAndDetails(
			stringField(errObj["message"]),
			pinnacleRcsErrorDataDetails(errObj),
		)
	}
	if status, _ := apiResponse["status"].(string); status == "failed" {
		details := ""
		if data, ok := apiResponse["data"].(map[string]interface{}); ok {
			details = stringField(data["details"])
		}
		return formatPinnacleRcsMessageAndDetails(stringField(apiResponse["message"]), details)
	}
	if msg := stringField(apiResponse["message"]); msg != "" {
		return msg
	}
	return "failed to send RCS message"
}

func pinnacleRcsErrorDataDetails(errObj map[string]interface{}) string {
	ed, ok := errObj["error_data"].(map[string]interface{})
	if !ok {
		return ""
	}
	return stringField(ed["details"])
}

func formatPinnacleRcsMessageAndDetails(message, details string) string {
	message = strings.TrimSpace(message)
	details = strings.TrimSpace(details)
	if message == "" && details == "" {
		return "failed to send RCS message"
	}
	if details == "" {
		return message
	}
	if message == "" {
		return details
	}
	return fmt.Sprintf("%s, details: %s", message, details)
}

func stringField(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
