package pinnacleWhatsappPayload

import (
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func GetPinnacleMediaPayload(pinnacleApiModel extapimodels.WhatsappRequestBody) map[string]interface{} {
	var buttonURL string

	if strings.Contains(pinnacleApiModel.Process, "poonawalla") {
		buttonURL = strings.Replace(pinnacleApiModel.ButtonLink, "<mobile>", pinnacleApiModel.Mobile[len(pinnacleApiModel.Mobile)-5:]+pinnacleApiModel.Mobile[:5], 1)
	} else {
		buttonURL = strings.Replace(pinnacleApiModel.ButtonLink, "<mobile>", pinnacleApiModel.Mobile, 1)
	}

	return map[string]interface{}{
		"recipient_type": "individual",
		"to":             pinnacleApiModel.Mobile,
		"type":           "image",
		"image": map[string]interface{}{
			"link": pinnacleApiModel.ImageUrl,
		},
		"metadata": map[string]interface{}{
			"messageId": strconv.Itoa(helper.GenerateRandomID(100000, 999999)), //TODO: Idempotency key
			"trackingCta": map[string]interface{}{
				"target": buttonURL,
			},
		},
	}
}
