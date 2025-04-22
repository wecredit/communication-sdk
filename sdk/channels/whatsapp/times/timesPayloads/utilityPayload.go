package timespayloads

import (
	"fmt"
	"strings"

	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
)

func GetTimesUtilityPayload(timesApiModel extapimodels.TimesAPIModel) (map[string]interface{}, error) {
	buttonURL := timesApiModel.ButtonLink

	fmt.Println("Process: ", timesApiModel.Process)

	// Handling For Dynamic Link
	if strings.Contains(buttonURL, "<mobile>") {
		// Handling For Poonawalla
		if strings.Contains(timesApiModel.Process, "indusind_holi") {
			// Modify the mobile format
			buttonURL = fmt.Sprintf("WA%s", strings.Replace(timesApiModel.ButtonLink, "<mobile>", timesApiModel.Mobile[len(timesApiModel.Mobile)-5:]+timesApiModel.Mobile[:5], 1))
		} else {
			buttonURL = strings.Replace(timesApiModel.ButtonLink, "<mobile>", timesApiModel.Mobile, 1)
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
				"components": []map[string]interface{}{
					{
						"type":     "button",
						"index":    "0",
						"sub_type": "url",
						"parameters": []map[string]interface{}{
							{
								"text": buttonURL,
								"type": "text",
							},
						},
					},
				},
			},
		}, nil

	} else {
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
				"components": []map[string]interface{}{}, // Empty components
			},
		}, nil
	}

}
