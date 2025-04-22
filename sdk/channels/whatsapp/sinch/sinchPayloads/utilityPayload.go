package sinchpayloads

import (
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/sdk/internal/models/extApiModels"
)

func GetSinchUtilityPayload(sinchApiModel extapimodels.SinchAPIModel) map[string]interface{} {
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
					"type":     "button",
					"index":    "0",
					"sub_type": "url",
					"parameters": []map[string]interface{}{
						{"type": "text", "text": buttonURL},
					},
				},
			},
		},
		"metadata": map[string]interface{}{
			"messageId": strconv.Itoa(helper.GenerateRandomID(100000, 999999)),
			"trackingCta": map[string]interface{}{
				"target": buttonURL,
				"tags": map[string]interface{}{
					"appID":    "wecreditpd",
					"template": sinchApiModel.TemplateName,
					"campaign": strings.ToUpper(sinchApiModel.Process),
					"MSISDN":   sinchApiModel.Mobile,
				},
			},
			"transactionId":  strconv.Itoa(helper.GenerateRandomID(100, 999)),
			"callbackDlrUrl": config.Configs.SinchCallbackURL,
			"media": map[string]interface{}{
				"mimeType": "image/jpeg",
			},
		},
	}

}
