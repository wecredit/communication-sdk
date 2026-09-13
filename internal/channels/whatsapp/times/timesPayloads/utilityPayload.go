package timesWhatsappPayload

import (
	"fmt"
	"strings"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	whatsappPayload "github.com/wecredit/communication-sdk/internal/channels/whatsapp/whatsappPayload"
)

func GetTimesUtilityPayload(timesApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	buttonURL := timesApiModel.ButtonLink
	mobileSub := whatsappPayload.ButtonMobileSubstitute(timesApiModel)

	fmt.Println("Process: ", timesApiModel.Process)

	components := []map[string]interface{}{}
	if positional := whatsappPayload.PositionalBodyParams(timesApiModel.TemplateVariableValues); len(positional) > 0 {
		components = append(components, map[string]interface{}{
			"type":       "body",
			"parameters": positional,
		})
	}

	// Handling For Dynamic Link
	if strings.Contains(buttonURL, "<mobile>") {
		if strings.Contains(timesApiModel.Process, "indusind_holi") {
			buttonURL = fmt.Sprintf("WA%s", strings.Replace(timesApiModel.ButtonLink, "<mobile>", mobileSub[len(mobileSub)-5:]+mobileSub[:5], 1))
		} else {
			buttonURL = strings.Replace(timesApiModel.ButtonLink, "<mobile>", mobileSub, 1)
		}

		components = append(components, map[string]interface{}{
			"type":     "button",
			"index":    "0",
			"sub_type": "url",
			"parameters": []map[string]interface{}{
				{"type": "text", "text": buttonURL},
			},
		})
	}

	return map[string]interface{}{
		"to":                timesApiModel.Mobile,
		"type":              "template",
		"recipient_type":    "individual",
		"messaging_product": "whatsapp",
		"template": map[string]interface{}{
			"name": timesApiModel.TemplateName,
			"language": map[string]interface{}{
				"code": "en_us",
			},
			"components": components,
		},
	}, nil
}
