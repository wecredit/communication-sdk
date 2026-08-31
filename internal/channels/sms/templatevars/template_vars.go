package templatevars

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

var varPlaceholder = regexp.MustCompile(`\{#var#\}`)

// ApplyTemplateVariables replaces ordered {#var#} placeholders using TemplateVariables
// names and values from the SMS request (named payload fields and/or TemplateVariableValues CSV).
// Vendor-agnostic: Sinch, Pinnacle, Times, and any future SMS provider share this path.
func ApplyTemplateVariables(data extapimodels.SmsRequestBody) (string, error) {
	if !strings.Contains(data.TemplateText, "{#var#}") {
		return data.TemplateText, nil
	}

	extras, err := overlayPositionalTemplateValues(&data)
	if err != nil {
		return "", err
	}
	keys := splitCSV(data.TemplateVariables)
	variableMap := map[string]string{
		"EmiAmount":         data.EmiAmount,
		"ApplicationNumber": data.ApplicationNumber,
		"CustomerName":      data.CustomerName,
		"LoanId":            data.LoanId,
		"PaymentLink":       data.PaymentLink,
		"Link":              data.PaymentLink,
		"Description":       data.Description,
	}
	for key, value := range extras {
		variableMap[key] = value
	}

	keyIndex := 0
	var replacementErr error

	text := varPlaceholder.ReplaceAllStringFunc(data.TemplateText, func(_ string) string {
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
			// Dynamic DLT slots (urg, args, ...) may be intentionally blank.
			return lookupTemplateVar(variableMap, key)
		}
	})

	if replacementErr != nil {
		return "", replacementErr
	}
	return text, nil
}

func lookupTemplateVar(variableMap map[string]string, key string) string {
	if value, ok := variableMap[key]; ok {
		return value
	}
	for mapKey, mapValue := range variableMap {
		if strings.EqualFold(mapKey, key) {
			return mapValue
		}
	}
	return ""
}

// overlayPositionalTemplateValues fills named fields from CommMarketingInput.VariablesValue.
// ZapCash/legacy already set CustomerName/PaymentLink/etc and leave TemplateVariableValues empty.
// Unknown names (urg, args, ...) stay in extras and fill {#var#} in TemplateVariables order.
func overlayPositionalTemplateValues(data *extapimodels.SmsRequestBody) (map[string]string, error) {
	extras := map[string]string{}
	if data == nil || strings.TrimSpace(data.TemplateVariableValues) == "" {
		return extras, nil
	}
	keys := splitCSV(data.TemplateVariables)
	values := splitCSV(data.TemplateVariableValues)
	if len(keys) != len(values) {
		return nil, fmt.Errorf("template variable count does not match supplied values")
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
			extras[key] = value
		}
	}
	return extras, nil
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
