package sinchpayloads

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

func GetTemplatePayload(data extapimodels.SinchSmsPayload, config models.Config) (map[string]interface{}, error) {
	templatePayload := map[string]interface{}{
		"userId":      config.SinchSmsApiUserName,
		"pass":        config.SinchSmsApiPassword,
		"appid":       config.SinchSmsApiAppID,
		"to":          fmt.Sprintf("91%s", verifyMobile(data.Mobile)),
		"from":        config.SinchSmsApiSender,
		"contenttype": "1",
		"selfid":      "true",
		"text":        data.TemplateText,
		"brd":         data.Process,                          // campaignName
		"dtm":         fmt.Sprintf("%d", data.DltTemplateId), // DLT Template ID
		"tc":          "4",                                   // Template Category : Service Explicit
		"intflag":     "false",
		"alert":       "1",
		"s":           "1", // Enable URL Shortening
	}
	return templatePayload, nil
}
