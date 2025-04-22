package timespayloads

import (
	"fmt"

	models "github.com/wecredit/communication-sdk/sdk/models"
)

func verifyMobile(mobile string) string {
	if len(mobile) == 10 {
		return mobile
	}
	return ""
}

func GetTemplatePayload(config models.Config) (map[string]interface{}, error) {
	templatePayload := map[string]interface{}{
		"extra": map[string]string{
			"dltContentId": config.TimesSmsDltContentId,
		},
		"message": map[string]string{
			"recipient": fmt.Sprintf("91%s", verifyMobile("8003366950")),
			"text":      fmt.Sprintf("Dear Customer. Login to WeCredit using OTP . WeCredit"),
		},
		"sender":  config.TimesSmsApiSender,
		"unicode": "False",
	}
	return templatePayload, nil
}
