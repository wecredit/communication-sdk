package timesWhatsappPayload

import (
	"strings"

	whatsappPayload "github.com/wecredit/communication-sdk/internal/channels/whatsapp/whatsappPayload"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func GetTimesMediaPayload(timesApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	buttonURL := timesApiModel.ButtonLink
	mobileSub := whatsappPayload.ButtonMobileSubstitute(timesApiModel)

	components := []map[string]interface{}{
		{
			"type": "header",
			"parameters": []map[string]interface{}{
				{
					"type": "image",
					"image": map[string]interface{}{
						"link": timesApiModel.ImageUrl,
					},
				},
			},
		},
	}
	if positional := whatsappPayload.PositionalBodyParams(timesApiModel.TemplateVariableValues); len(positional) > 0 {
		components = append(components, map[string]interface{}{
			"type":       "body",
			"parameters": positional,
		})
	}

	if strings.Contains(buttonURL, "<mobile>") {
		if strings.Contains(timesApiModel.Process, "indusind_holi") {
			buttonURL = "WA" + strings.Replace(timesApiModel.ButtonLink, "<mobile>", mobileSub[len(mobileSub)-5:]+mobileSub[:5], 1)
		} else {
			buttonURL = mobileSub
		}

		if timesApiModel.Process == "lnt" {
			components = append(components,
				map[string]interface{}{
					"type":     "button",
					"sub_type": "url",
					"index":    "0",
					"parameters": []map[string]interface{}{
						{"type": "text", "text": timesApiModel.Mobile},
					},
				},
				map[string]interface{}{
					"type":     "button",
					"sub_type": "url",
					"index":    "1",
					"parameters": []map[string]interface{}{
						{"type": "text", "text": timesApiModel.Mobile},
					},
				},
			)
		} else {
			components = append(components, map[string]interface{}{
				"type":     "button",
				"sub_type": "url",
				"index":    "0",
				"parameters": []map[string]interface{}{
					{"type": "text", "text": buttonURL},
				},
			})
		}
	}

	return map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                timesApiModel.Mobile,
		"type":              "template",
		"template": map[string]interface{}{
			"name": timesApiModel.TemplateName,
			"language": map[string]interface{}{
				"code": "en_us",
			},
			"components": components,
		},
	}, nil
}
