package sinchWhatsappPayload

import (
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func GetSinchMediaPayload(sinchApiModel extapimodels.WhatsappRequestBody) map[string]interface{} {
	var buttonURL string

	if strings.Contains(sinchApiModel.Process, "poonawalla") {
		// Modify the mobile format
		buttonURL = strings.Replace(sinchApiModel.ButtonLink, "<mobile>", sinchApiModel.Mobile[len(sinchApiModel.Mobile)-5:]+sinchApiModel.Mobile[:5], 1)
	} else {
		buttonURL = strings.Replace(sinchApiModel.ButtonLink, "<mobile>", sinchApiModel.Mobile, 1)
	}

	return map[string]interface{}{
		"recipient_type": "individual",
		"to":             sinchApiModel.Mobile,
		"type":           "template",
		"template": map[string]interface{}{
			"name": sinchApiModel.TemplateName,
			"language": map[string]interface{}{
				"policy": "deterministic",
				"code":   "en_US",
			},
			"components": []map[string]interface{}{
				{
					"type": "header",
					"parameters": []map[string]interface{}{
						{
							"type": "image",
							"image": map[string]interface{}{
								"id": sinchApiModel.ImageID,
							},
						},
					},
				},
				{
					"type":     "button",
					"index":    "0",
					"sub_type": "url",
					"parameters": []map[string]interface{}{
						{
							"type": "text",
							"text": buttonURL,
						},
					},
				},
			},
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

}
