package timesWhatsappPayload

import (
	"fmt"
	"strings"

	whatsappPayload "github.com/wecredit/communication-sdk/internal/channels/whatsapp/whatsappPayload"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func GetTimesUtilityPayload(timesApiModel extapimodels.WhatsappRequestBody) (map[string]interface{}, error) {
	buttonURL := timesApiModel.ButtonLink
	mobileSub := whatsappPayload.ButtonMobileSubstitute(timesApiModel)

	fmt.Println("Process: ", timesApiModel.Process)

	components := []map[string]interface{}{}
	if bodyParams := whatsappPayload.BodyParams(timesApiModel); len(bodyParams) > 0 {
		components = append(components, map[string]interface{}{
			"type":       "body",
			"parameters": bodyParams,
		})
	}

	// Hermis Times utility: button text is DynamicMobile when <mobile> is present.
	if strings.Contains(buttonURL, "<mobile>") {
		buttonURL = mobileSub
		// Legacy SDK-only indusind_holi digit rotation + WA prefix (not in hermis). Kept for reference:
		// if strings.Contains(timesApiModel.Process, "indusind_holi") {
		// 	buttonURL = fmt.Sprintf("WA%s", strings.Replace(timesApiModel.ButtonLink, "<mobile>", mobileSub[len(mobileSub)-5:]+mobileSub[:5], 1))
		// } else {
		// 	buttonURL = strings.Replace(timesApiModel.ButtonLink, "<mobile>", mobileSub, 1)
		// }

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
