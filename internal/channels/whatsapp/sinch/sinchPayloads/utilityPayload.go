package sinchWhatsappPayload

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func GetSinchUtilityPayload(sinchApiModel extapimodels.WhatsappRequestBody) map[string]interface{} {
	var buttonURL string

	if strings.Contains(sinchApiModel.Process, "poonawalla") {
		buttonURL = strings.Replace(sinchApiModel.ButtonLink, "<mobile>", sinchApiModel.Mobile[len(sinchApiModel.Mobile)-5:]+sinchApiModel.Mobile[:5], 1)
	} else {
		buttonURL = strings.Replace(sinchApiModel.ButtonLink, "<mobile>", sinchApiModel.Mobile, 1)
	}

	var components []map[string]interface{}

	// Add dynamic body parameters based on known struct fields
	if sinchApiModel.Client == variables.CreditSea && sinchApiModel.TemplateVariables != "" {
		keys := strings.Split(sinchApiModel.TemplateVariables, ",")
		for _, key := range keys {
			key = strings.TrimSpace(key)
			var textValue string

			switch key {
			case "CustomerName":
				textValue = sinchApiModel.CustomerName
			case "DueDate":
				dueDateStr := sinchApiModel.DueDate
				var formatted string
				var parsed bool

				formatted = dueDateStr // Default to the original string if parsing fails

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
					utils.Error(fmt.Errorf("invalid DueDate format: %s", dueDateStr))
				}
				textValue = formatted
			case "LoanId":
				textValue = sinchApiModel.LoanId
			case "ApplicationNumber":
				textValue = sinchApiModel.ApplicationNumber
			case "EmiAmount":
				textValue = sinchApiModel.EmiAmount
			default:
				textValue = "" // skip unknown fields
			}

			if textValue != "" {
				components = append(components, map[string]interface{}{
					"type": "body",
					"parameters": []map[string]interface{}{
						{
							"type": "text",
							"text": textValue,
						},
					},
				})
			}
		}
	}

	// Add the button component
	components = append(components, map[string]interface{}{
		"type":     "button",
		"index":    "0",
		"sub_type": "url",
		"parameters": []map[string]interface{}{
			{"type": "text", "text": buttonURL},
		},
	})

	// Final payload
	templatePayload := map[string]interface{}{
		"recipient_type": "individual",
		"to":             sinchApiModel.Mobile,
		"type":           "template",
		"template": map[string]interface{}{
			"name": sinchApiModel.TemplateName,
			"language": map[string]interface{}{
				"policy": "deterministic",
				"code":   "en_US",
			},
			"components": components,
		},
		"metadata": map[string]interface{}{
			"messageId": strconv.Itoa(helper.GenerateRandomID(100000, 999999)),
			"trackingCta": map[string]interface{}{
				"target": buttonURL,
				"tags": map[string]interface{}{
					"appID":    sinchApiModel.AppId,
					"template": sinchApiModel.TemplateName,
					"campaign": strings.ToUpper(sinchApiModel.Process),
					"MSISDN":   sinchApiModel.Mobile,
				},
			},
			"transactionId":  strconv.Itoa(helper.GenerateRandomID(100, 999)),
			"callbackDlrUrl": config.Configs.SinchWhatsappCallbackURL,
			"media": map[string]interface{}{
				"mimeType": "image/jpeg",
			},
		},
	}

	return templatePayload
}
