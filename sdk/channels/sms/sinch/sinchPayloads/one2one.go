package sinchpayloads

import (
	"fmt"
	"os"

	models "github.com/wecredit/communication-sdk/sdk/models"
)

func verifyMobile(mobile string) string {
	if len(mobile) == 10 {
		return mobile
	}
	return ""
}

func GetTemplatePayload(recpients string, config models.Config) (map[string]interface{}, error) {
	templatePayload := map[string]interface{}{
		"userId":      config.SinchSmsApiUserName,
		"pass":        config.SinchSmsApiPassword,
		"appid":       config.SinchSmsApiAppID,
		"to":          fmt.Sprintf("91%s", verifyMobile(recpients)),
		"from":        os.Getenv("sms_api_sender"),
		"contenttype": "1",
		"selfid":      "true",
		"text":        "Ram Fincorp se lein Rs 1,00,000 tak ka loan bina foreclosure charge ke. Abhi App download karein. https://play.google.com/store/apps/details?id=com.ramfincorploan  WeCredit",
		"brd":         "",                          // campaignName
		"dtm":         config.SinchSmsDltContentId, // DLT Template ID
		"tc":          "4",                         // Template Category : Service Explicit
		"intflag":     "false",
		"alert":       "1",
		"s":           "1", // Enable URL Shortening
	}
	return templatePayload, nil
}
