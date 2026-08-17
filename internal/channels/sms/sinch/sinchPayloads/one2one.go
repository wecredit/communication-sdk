package sinchSmsPayload

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	models "github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func verifyMobile(mobile string) string {
	if len(mobile) == 10 {
		return mobile
	}
	return ""
}

func GetTemplatePayload(data extapimodels.SmsRequestBody, config models.Config) (map[string]interface{}, error) {
	username, password, appId, sender, err := SinchSMSCredentials(data.Client, config)
	if err != nil {
		return nil, err
	}
	if header := strings.TrimSpace(data.TemplateHeader); header != "" {
		sender = header
	}

	if strings.Contains(data.TemplateText, "{#var#}") {
		var err error
		data.TemplateText, err = ApplySinchTemplateVariables(data)
		if err != nil {
			return nil, err
		}
	}

	templatePayload := map[string]interface{}{
		"alert":       "1",
		"appid":       appId,
		"brd":         fmt.Sprintf("%s_%s", data.Process, data.Description), // campaignName
		"contenttype": "1",
		"dtm":         fmt.Sprintf("%d", data.DltTemplateId), // DLT Template ID
		"from":        sender,
		"intflag":     "false",
		"pass":        password,
		"s":           "1", // Enable URL Shortening
		"selfid":      "true",
		"tc":          data.TemplateCategory, // Template Category : Service Explicit (4) or Implicit (3)
		"text":        data.TemplateText,
		"to":          fmt.Sprintf("91%s", verifyMobile(data.Mobile)),
		"userId":      username,
	}

	return templatePayload, nil
}

func ApplySinchTemplateVariables(data extapimodels.SmsRequestBody) (string, error) {
	if err := overlayPositionalTemplateValues(&data); err != nil {
		return "", err
	}
	keys := strings.Split(data.TemplateVariables, ",")
	variableMap := map[string]string{
		"EmiAmount":         data.EmiAmount,
		"ApplicationNumber": data.ApplicationNumber,
		"CustomerName":      data.CustomerName,
		"LoanId":            data.LoanId,
		"PaymentLink":       data.PaymentLink,
		"Link":              data.PaymentLink,
		"Description":       data.Description,
	}

	keyIndex := 0
	re := regexp.MustCompile(`\{#var#\}`)
	var replacementErr error

	text := re.ReplaceAllStringFunc(data.TemplateText, func(_ string) string {
		if replacementErr != nil {
			return ""
		}
		if keyIndex >= len(keys) {
			replacementErr = fmt.Errorf("template has more {#var#} placeholders than TemplateVariables entries")
			return ""
		}

		key := strings.TrimSpace(keys[keyIndex])
		keyIndex++

		switch key {
		case "CustomerName":
			textValue := data.CustomerName
			if textValue == "" {
				textValue = "Dear Customer"
			}
			return textValue

		case "DueDate":
					// Only process if not already formatted
			if _, ok := variableMap["DueDate"]; !ok {
				dueDateStr := data.DueDate
				var formatted string
				var parsed bool

				layouts := []string{
					time.RFC3339,
					"2006-01-02 15:04:05 -0700 MST",
					"2006-01-02 15:04:05",
					"2006-01-02",
				}

				for _, layout := range layouts {
					if t, err := time.Parse(layout, dueDateStr); err == nil {
						formatted = t.Format("2006-01-02")
						parsed = true
						break
					}
				}

				if !parsed || strings.TrimSpace(formatted) == "" {
					replacementErr = fmt.Errorf("invalid DueDate format: %s", dueDateStr)
					return ""
				}

				variableMap["DueDate"] = formatted
			}

			return variableMap["DueDate"]

		case "EmiAmount":
			value := strings.TrimSpace(variableMap["EmiAmount"])
			if value == "" || value == "0" || value == "0.0" {
				replacementErr = fmt.Errorf("missing value for required variable: %s", key)
				return ""
			}
			return value

		case "PaymentLink", "Link":
			value := strings.TrimSpace(variableMap["PaymentLink"])
			if value == "" {
				replacementErr = fmt.Errorf("missing value for required variable: %s", key)
				return ""
			}
			return value

		default:
			value := strings.TrimSpace(variableMap[key])
			if value == "" {
				replacementErr = fmt.Errorf("missing value for required variable: %s", key)
				return ""
			}
			return value
		}
	})

	if replacementErr != nil {
		return "", replacementErr
	}
	return text, nil
}

// overlayPositionalTemplateValues fills named fields from CommMarketingInput.VariablesValue.
// ZapCash/legacy already set CustomerName/PaymentLink/etc and leave TemplateVariableValues empty.
func overlayPositionalTemplateValues(data *extapimodels.SmsRequestBody) error {
	if data == nil || strings.TrimSpace(data.TemplateVariableValues) == "" {
		return nil
	}
	keys := splitCSV(data.TemplateVariables)
	values := splitCSV(data.TemplateVariableValues)
	if len(keys) != len(values) {
		return fmt.Errorf("template variable count does not match supplied values")
	}
	for i, key := range keys {
		value := values[i]
		switch strings.ToLower(key) {
		case "customername":
			data.CustomerName = value
		case "emiamount":
			data.EmiAmount = value
		case "loanid":
			data.LoanId = value
		case "applicationnumber":
			data.ApplicationNumber = value
		case "duedate":
			data.DueDate = value
		case "description":
			data.Description = value
		case "paymentlink", "link":
			data.PaymentLink = value
		default:
			return fmt.Errorf("unsupported template variable: %s", key)
		}
	}
	return nil
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func SinchSMSCredentials(client string, config models.Config) (username, password, appID, sender string, err error) {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case "wecredit":
		username, password, appID, sender = config.SinchSmsApiUserName, config.SinchSmsApiPassword, config.SinchSmsApiAppID, config.SinchSmsApiSender
	case variables.CreditSea:
		username, password, appID, sender = config.CreditSeaSinchSmsApiUserName, config.CreditSeaSinchSmsApiPassword, config.CreditSeaSinchSmsApiAppID, config.CreditSeaSinchSmsApiSender
	default:
		return "", "", "", "", fmt.Errorf("Sinch SMS credentials are not configured for client: %s", client)
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" || strings.TrimSpace(appID) == "" || strings.TrimSpace(sender) == "" {
		return "", "", "", "", fmt.Errorf("Sinch SMS credentials are incomplete for client: %s", client)
	}
	return username, password, appID, sender, nil
}
