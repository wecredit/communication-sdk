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
	var username, password, appId, sender string
	if data.Client == variables.CreditSea {
		username = config.CreditSeaSinchSmsApiUserName
		password = config.CreditSeaSinchSmsApiPassword
		appId = config.CreditSeaSinchSmsApiAppID
		sender = config.CreditSeaSinchSmsApiSender

		if strings.Contains(data.TemplateText, "{#var#}") {
			var keys = strings.Split(data.TemplateVariables, ",")
			fmt.Println("Keys for template variables:", keys)

			// Lookup map with only non-derived values first
			variableMap := map[string]string{
				"EmiAmount":         data.EmiAmount,
				"ApplicationNumber": data.ApplicationNumber,
				"CustomerName":      data.CustomerName,
				"LoanId":            data.LoanId,
			}

			keyIndex := 0
			re := regexp.MustCompile(`\{#var#\}`)

			var replacementErr error // capture error to return after ReplaceAllStringFunc

			data.TemplateText = re.ReplaceAllStringFunc(data.TemplateText, func(_ string) string {
				if keyIndex >= len(keys) || replacementErr != nil {
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

						var layouts = []string{
							time.RFC3339,                    // "2025-06-08T00:00:00Z"
							"2006-01-02 15:04:05 -0700 MST", // Go's full time format with timezone
							"2006-01-02 15:04:05",           // Datetime without timezone
							"2006-01-02",                    // 🆕 Date-only
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

				default:
					return strings.TrimSpace(variableMap[key])
				}
			})
			// If error occurred during replacement, return early
			if replacementErr != nil {
				return nil, replacementErr
			}
		}
	} else {
		username = config.SinchSmsApiUserName
		password = config.SinchSmsApiPassword
		appId = config.SinchSmsApiAppID
		sender = config.SinchSmsApiSender
	}

	templatePayload := map[string]interface{}{
		"userId":      username,
		"pass":        password,
		"appid":       appId,
		"to":          fmt.Sprintf("91%s", verifyMobile(data.Mobile)),
		"from":        sender,
		"contenttype": "1",
		"selfid":      "true",
		"text":        data.TemplateText,
		"brd":         fmt.Sprintf("%s_%s", data.Process, data.Description), // campaignName
		"dtm":         fmt.Sprintf("%d", data.DltTemplateId),                // DLT Template ID
		"tc":          data.TemplateCategory,                                // Template Category : Service Explicit (4) or Implicit (3)
		"intflag":     "false",
		"alert":       "1",
		"s":           "1", // Enable URL Shortening
	}

	// if data.Client != variables.CreditSea {
	// 	templatePayload["s"] = "1"
	// }

	fmt.Println("Sinch SMS Template Payload:", templatePayload)

	return templatePayload, nil
}
