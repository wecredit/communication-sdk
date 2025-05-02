package timesWhatsappPayload

import (
	"strings"

	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
)

func GetTimesMediaPayload(timesApiModel extapimodels.TimesAPIModel) (map[string]interface{}, error) {
	buttonURL := timesApiModel.ButtonLink

	// Handling For Dynamic Link
	if strings.Contains(buttonURL, "<mobile>") {

		// Handling For Poonawalla
		if strings.Contains(timesApiModel.Process, "indusind_holi") {
			// Modify the mobile format
			buttonURL = "WA" + strings.Replace(timesApiModel.ButtonLink, "<mobile>", timesApiModel.Mobile[len(timesApiModel.Mobile)-5:]+timesApiModel.Mobile[:5], 1)
		} else {
			buttonURL = timesApiModel.Mobile
		}

		if timesApiModel.Process == "lnt" {
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
					"components": []map[string]interface{}{
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
						{
							"type":     "button",
							"sub_type": "url",
							"index":    "0",
							"parameters": []map[string]interface{}{
								{
									"type": "text",
									"text": timesApiModel.Mobile,
								},
							},
						},
						{
							"type":     "button",
							"sub_type": "url",
							"index":    "1",
							"parameters": []map[string]interface{}{
								{
									"type": "text",
									"text": timesApiModel.Mobile,
								},
							},
						},
					},
				},
			}, nil
		} else {
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
					"components": []map[string]interface{}{
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
						{
							"type":     "button",
							"sub_type": "url",
							"index":    "0",
							"parameters": []map[string]interface{}{
								{
									"type": "text",
									"text": buttonURL,
								},
							},
						},
					},
				},
			}, nil
		}

	} else {
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
				"components": []map[string]interface{}{
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
				},
			},
		}, nil
	}

}
