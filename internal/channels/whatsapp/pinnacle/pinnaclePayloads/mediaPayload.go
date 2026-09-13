package pinnacleWhatsappPayload

import (
	"fmt"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/helper"
	whatsappPayload "github.com/wecredit/communication-sdk/internal/channels/whatsapp/whatsappPayload"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func GetPinnacleMediaPayload(pinnacleApiModel extapimodels.WhatsappRequestBody) map[string]interface{} {
	var buttonURL string
	mobileSub := whatsappPayload.ButtonMobileSubstitute(pinnacleApiModel)

	// Customize the mobile number for poonawalla if required
	if strings.Contains(pinnacleApiModel.Process, "poonawalla") {
		buttonURL = strings.Replace(pinnacleApiModel.ButtonLink, "<mobile>", mobileSub[len(mobileSub)-5:]+mobileSub[:5], 1)
	} else {
		buttonURL = strings.Replace(pinnacleApiModel.ButtonLink, "<mobile>", mobileSub, 1)
	}

	var components []map[string]interface{}
	var bodyParams []map[string]interface{}

	languageCode := strings.TrimSpace(pinnacleApiModel.LanguageCode)
	if languageCode == "" {
		languageCode = "en_US"
	}

	if positional := whatsappPayload.PositionalBodyParams(pinnacleApiModel.TemplateVariableValues); len(positional) > 0 {
		bodyParams = positional
	} else if pinnacleApiModel.TemplateVariables != "" {
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

	if pinnacleApiModel.ImageUrl != "" {
		components = append(components, map[string]interface{}{
			"type": "header",
			"parameters": []map[string]interface{}{
				{
					"type": "image",
					"image": map[string]interface{}{
						"link": pinnacleApiModel.ImageUrl,
					},
				},
			},
		})
	}

	// Add body component only once with all parameters
	if len(bodyParams) > 0 {
		components = append(components, map[string]interface{}{
			"type":       "body",
			"parameters": bodyParams,
		})
	}

	waba := strings.TrimSpace(pinnacleApiModel.WabaNumber)
	if waba == "" {
		waba = config.Configs.PinnacleZapcashWabaId
	}

	campaignID := strings.TrimSpace(pinnacleApiModel.CampaignId)
	if campaignID == "" {
		campaignID = "0"
	}

	ctaID := strings.TrimSpace(pinnacleApiModel.CtaId)
	if ctaID == "" {
		ctaID = "1"
	}

	// Add the button component (hermis CTA path when WABA known)
	buttonPayload := buttonURL
	if waba != "" {
		buttonPayload = fmt.Sprintf("cta/%s/%s/%s/%s/%s", waba, pinnacleApiModel.Mobile, campaignID, ctaID, buttonURL)
	}

	components = append(components, map[string]interface{}{
		"type":     "button",
		"index":    "0",
		"sub_type": "url",
		"parameters": []map[string]interface{}{
			{
				"type":    "payload",
				"payload": buttonPayload,
			},
		},
	})

	leadID := pinnacleApiModel.CommId
	if strings.TrimSpace(leadID) == "" {
		leadID = fmt.Sprintf("zap_%d", helper.GenerateRandomID(10000000, 99999999))
	}

	// Build the full payload
	templatePayload := map[string]interface{}{
		"recipient_type":    "individual",
		"to":                pinnacleApiModel.Mobile,
		"type":              "template",
		"messaging_product": "whatsapp",
		"biz_opaque_callback_data": map[string]interface{}{
			"lead_id":  leadID,
			"campaign": pinnacleApiModel.Process,
			"source":   pinnacleApiModel.Client,
		},
		"template": map[string]interface{}{
			"name": pinnacleApiModel.TemplateName,
			"language": map[string]interface{}{
				"code": languageCode,
			},
			"components": components,
		},
	}

	return templatePayload
}
