package sinchSmsPayload

import (
	"fmt"

	models "github.com/wecredit/communication-sdk/sdk/models"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func verifyMobile(mobile string) string {
	if len(mobile) == 10 {
		return mobile
	}
	return ""
}

func GetTemplatePayload(data extapimodels.SmsRequestBody, config models.Config) (map[string]interface{}, error) {
	var username, password, appId, sender string
	if data.Client == variables.CreditSea {
		username = config.CreditSeaSinchSmsApiUserName
		password = config.CreditSeaSinchSmsApiPassword
		appId = config.CreditSeaSinchSmsApiAppID
		sender = config.CreditSeaSinchSmsApiSender
	} else {
		username = config.SinchSmsApiUserName
		password = config.SinchSmsApiPassword
		appId = config.SinchSmsApiAppID
		sender = config.SinchSmsApiSender
	}

	templatePayload := map[string]interface{}{
		"userId":      username,
		"pass":        password,
		"appid":       appId,
		"to":          fmt.Sprintf("91%s", verifyMobile(data.Mobile)),
		"from":        sender,
		"contenttype": "1",
		"selfid":      "true",
		"text":        data.TemplateText,
		"brd":         data.Process,                          // campaignName
		"dtm":         fmt.Sprintf("%d", data.DltTemplateId), // DLT Template ID
		"tc":          data.TemplateCategory,                 // Template Category : Service Explicit (4) or Implicit (3)
		"intflag":     "false",
		"alert":       "1",
		// "s":           "1", // Enable URL Shortening
	}

	if data.Client != variables.CreditSea {
		templatePayload["s"] = "1"
	}

	return templatePayload, nil
}
