package sinchSmsPayload

import (
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/internal/channels/sms/templatevars"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	models "github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func verifyMobile(mobile string) string {
	if len(mobile) == 10 {
		return mobile
	}
	return ""
}

func GetTemplatePayload(data extapimodels.SmsRequestBody, config models.Config) (map[string]interface{}, error) {
	username, password, appId, sender, err := SinchSMSCredentials(data.Client, config)
	if err != nil {
		return nil, err
	}
	if header := strings.TrimSpace(data.TemplateHeader); header != "" {
		sender = header
	}

	// {#var#} substitution is applied once in sms.SendSmsByProcess via templatevars
	// before any vendor call. Payload builders must not re-apply.

	templatePayload := map[string]interface{}{
		"alert":       "1",
		"appid":       appId,
		"brd":         fmt.Sprintf("%s_%s", data.Process, data.Description), // campaignName
		"contenttype": "1",
		"dtm":         fmt.Sprintf("%d", data.DltTemplateId), // DLT Template ID
		"from":        sender,
		"intflag":     "false",
		"pass":        password,
		"s":           "1", // Enable URL Shortening
		"selfid":      "true",
		"tc":          data.TemplateCategory, // Template Category : Service Explicit (4) or Implicit (3)
		"text":        data.TemplateText,
		"to":          fmt.Sprintf("91%s", verifyMobile(data.Mobile)),
		"userId":      username,
	}

	return templatePayload, nil
}

// ApplySinchTemplateVariables is retained as a thin wrapper for older call sites/tests.
// Prefer templatevars.ApplyTemplateVariables for new code.
func ApplySinchTemplateVariables(data extapimodels.SmsRequestBody) (string, error) {
	return templatevars.ApplyTemplateVariables(data)
}

func SinchSMSCredentials(client string, config models.Config) (username, password, appID, sender string, err error) {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case "wecredit":
		username, password, appID, sender = config.SinchSmsApiUserName, config.SinchSmsApiPassword, config.SinchSmsApiAppID, config.SinchSmsApiSender
	case variables.CreditSea:
		username, password, appID, sender = config.CreditSeaSinchSmsApiUserName, config.CreditSeaSinchSmsApiPassword, config.CreditSeaSinchSmsApiAppID, config.CreditSeaSinchSmsApiSender
	default:
		return "", "", "", "", fmt.Errorf("Sinch SMS credentials are not configured for client: %s", client)
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" || strings.TrimSpace(appID) == "" || strings.TrimSpace(sender) == "" {
		return "", "", "", "", fmt.Errorf("Sinch SMS credentials are incomplete for client: %s", client)
	}
	return username, password, appID, sender, nil
}
