package timesSmsPayload

import (
	"fmt"

	models "github.com/wecredit/communication-sdk/sdk/models"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
)

func verifyMobile(mobile string) string {
	if len(mobile) == 10 {
		return mobile
	}
	return ""
}

func GetTemplatePayload(data extapimodels.SmsRequestBody, config models.Config) (map[string]interface{}, error) {
	templatePayload := map[string]interface{}{
		"extra": map[string]string{
			"dltContentId": fmt.Sprintf("%d", data.DltTemplateId),
		},
		"message": map[string]string{
			"recipient": fmt.Sprintf("91%s", verifyMobile(data.Mobile)),
			"text":      data.TemplateText,
		},
		"sender":  config.TimesSmsApiSender,
		"unicode": "False",
	}
	return templatePayload, nil
}
