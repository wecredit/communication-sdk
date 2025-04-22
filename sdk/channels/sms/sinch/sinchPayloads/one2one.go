package sinchpayloads

import (
	"fmt"
	"os"

	models "dev.azure.com/wctec/communication-engine/sdk/internal/models"
)

func verifyMobile(mobile string) string {
	if len(mobile) == 10 {
		return mobile
	}
	return ""
}

func GetTemplatePayload(recpients string, config models.Config) (map[string]interface{}, error) {
	templatePayload := map[string]interface{}{
		"appid":       config.SinchSmsApiAppID,
		"userId":      config.SinchSmsApiUserName,
		"pass":        config.SinchSmsApiPassword,
		"contenttype": "1",
		"from":        os.Getenv("sms_api_sender"),
		"to":          fmt.Sprintf("91%s", verifyMobile(recpients)),
		"alert":       "1",
		"selfid":      "true",
		"intflag":     "false",
		"dtm":         config.SinchSmsDltContentId,
		"tc":          "4",
		"text":        "Ram Fincorp se lein Rs 1,00,000 tak ka loan bina foreclosure charge ke. Abhi App download karein. https://play.google.com/store/apps/details?id=com.ramfincorploan  WeCredit",
		"s":           "1",
	}
	return templatePayload, nil
}
