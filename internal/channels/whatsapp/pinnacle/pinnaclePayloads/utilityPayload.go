package pinnacleWhatsappPayload

import (
	"fmt"
	// "strconv"
	"strings"
	"time"

	// "github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func GetPinnacleUtilityPayload(pinnacleApiModel extapimodels.WhatsappRequestBody) map[string]interface{} {
	var buttonURL string

	// Customize the mobile number for poonawalla if required
	if strings.Contains(pinnacleApiModel.Process, "poonawalla") {
		buttonURL = strings.Replace(pinnacleApiModel.ButtonLink, "<mobile>", pinnacleApiModel.Mobile[len(pinnacleApiModel.Mobile)-5:]+pinnacleApiModel.Mobile[:5], 1)
	} else {
		buttonURL = strings.Replace(pinnacleApiModel.ButtonLink, "<mobile>", pinnacleApiModel.Mobile, 1)
	}

	var components []map[string]interface{}
	var bodyParams []map[string]interface{}

	// Add dynamic text values to a single body component
	if pinnacleApiModel.TemplateVariables != "" {
		keys := strings.Split(pinnacleApiModel.TemplateVariables, ",")
		for _, key := range keys {
			key = strings.TrimSpace(key)
			var textValue string

			switch key {
			case "CustomerName":
				textValue = pinnacleApiModel.CustomerName
				if textValue == "" {
					textValue = "Customer"
				}

			case "DueDate":
				dueDateStr := pinnacleApiModel.DueDate
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
				textValue = pinnacleApiModel.LoanId

			case "ApplicationNumber":
				textValue = pinnacleApiModel.ApplicationNumber

			case "EmiAmount":
				textValue = pinnacleApiModel.EmiAmount

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
				"type":    "payload",
				"payload": fmt.Sprintf("cta/919218115984/%s/0/%s", pinnacleApiModel.Mobile, buttonURL),
			},
		},
	})

	// Build the full payload
	templatePayload := map[string]interface{}{
		"recipient_type":    "individual",
		"to":                pinnacleApiModel.Mobile,
		"type":              "template",
		"messaging_product": "whatsapp",
		"biz_opaque_callback_data": map[string]interface{}{
			"lead_id":  fmt.Sprintf("zap_%d", helper.GenerateRandomID(10000000, 99999999)),
			"campaign": fmt.Sprintf("%s_%s", pinnacleApiModel.Process, pinnacleApiModel.Description),
			"source":   pinnacleApiModel.Client,
		},
		"template": map[string]interface{}{
			"name": pinnacleApiModel.TemplateName,
			"language": map[string]interface{}{
				"code": "en",
			},
			"components": components,
		},
		// "metadata": map[string]interface{}{
		// 	"messageId": strconv.Itoa(helper.GenerateRandomID(100000, 999999)), //TODO: Idempotency key
		// 	"trackingCta": map[string]interface{}{
		// 		"target": buttonURL,
		// 		"tags": map[string]interface{}{
		// 			"appID":    pinnacleApiModel.AppId,
		// 			"template": pinnacleApiModel.TemplateName,
		// 			"campaign": strings.ToUpper(pinnacleApiModel.Process),
		// 			"MSISDN":   pinnacleApiModel.Mobile,
		// 		},
		// 	},
		// 	"transactionId":  strconv.Itoa(helper.GenerateRandomID(100, 999)),
		// 	"callbackDlrUrl": config.Configs.SinchWhatsappCallbackURL,
		// 	"media": map[string]interface{}{
		// 		"mimeType": "image/jpeg",
		// 	},
		// },
	}

	return templatePayload
}
