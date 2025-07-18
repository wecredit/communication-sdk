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

	// Customize the mobile number for poonawalla if required
	if strings.Contains(sinchApiModel.Process, "poonawalla") {
		buttonURL = strings.Replace(sinchApiModel.ButtonLink, "<mobile>", sinchApiModel.Mobile[len(sinchApiModel.Mobile)-5:]+sinchApiModel.Mobile[:5], 1)
	} else {
		buttonURL = strings.Replace(sinchApiModel.ButtonLink, "<mobile>", sinchApiModel.Mobile, 1)
	}

	var components []map[string]interface{}
	var bodyParams []map[string]interface{}

	// Add dynamic text values to a single body component
	if sinchApiModel.Client == variables.CreditSea && sinchApiModel.TemplateVariables != "" {
		keys := strings.Split(sinchApiModel.TemplateVariables, ",")
		for _, key := range keys {
			key = strings.TrimSpace(key)
			var textValue string

			switch key {
			case "CustomerName":
				textValue = sinchApiModel.CustomerName
				if textValue == "" {
					textValue = "Customer"
				}

			case "DueDate":
				dueDateStr := sinchApiModel.DueDate
				formatted := dueDateStr // fallback
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

				if !parsed {
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
				textValue = "" // ignore unknown fields
			}

			if textValue != "" {
				bodyParams = append(bodyParams, map[string]interface{}{
					"type": "text",
					"text": textValue,
				})
			}
		}
	}

	// Add body component only once with all parameters
	if len(bodyParams) > 0 {
		components = append(components, map[string]interface{}{
			"type":       "body",
			"parameters": bodyParams,
		})
	}

	// Add the button component
	components = append(components, map[string]interface{}{
		"type":     "button",
		"index":    "0",
		"sub_type": "url",
		"parameters": []map[string]interface{}{
			{
				"type": "text",
				"text": buttonURL,
			},
		},
	})

	// Build the full payload
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
