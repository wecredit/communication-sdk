package vapioSmsPayload

import (
	"fmt"
	"strconv"
	"strings"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	models "github.com/wecredit/communication-sdk/sdk/models"
)

// GetTemplatePayload builds the Vapio form body.
// {#var#} substitution is applied once in sms.SendSmsByProcess via templatevars
// before any vendor call. Payload builders must not re-apply.
func GetTemplatePayload(data extapimodels.SmsRequestBody, cfg models.Config) (map[string]string, error) {
	username := strings.TrimSpace(cfg.WeCreditVapioSmsUsername)
	apiKey := strings.TrimSpace(cfg.WeCreditVapioSmsApiKey)
	senderID := strings.TrimSpace(data.TemplateHeader)
	route := strings.TrimSpace(cfg.WeCreditVapioSmsRoute)
	if username == "" || apiKey == "" || route == "" {
		return nil, fmt.Errorf("Vapio SMS credentials are incomplete")
	}
	if senderID == "" {
		return nil, fmt.Errorf("Vapio SMS template header is empty")
	}

	message := strings.TrimSpace(data.TemplateText)
	if message == "" {
		return nil, fmt.Errorf("Vapio SMS message text is empty")
	}

	if data.DltTemplateId <= 0 {
		return nil, fmt.Errorf("Vapio SMS DLT template id is missing")
	}

	mobile, err := NormalizeMobile10(data.Mobile)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"username": username,
		"apikey":   apiKey,
		"senderid": senderID,
		"route":    route,
		"mobile":   mobile,
		"format":   "json",
		"TID":      strconv.FormatInt(data.DltTemplateId, 10),
		"text":     message,
	}, nil
}

// NormalizeMobile10 returns a 10-digit national number.
// 91XXXXXXXXXX and 0XXXXXXXXXX are reduced the same way Pinnacle accepts them,
// then stored as the 10 digits Vapio expects.
func NormalizeMobile10(mobile string) (string, error) {
	digits := onlyDigits(mobile)
	switch {
	case len(digits) == 10:
		return digits, nil
	case len(digits) == 12 && strings.HasPrefix(digits, "91"):
		return digits[2:], nil
	case len(digits) == 11 && strings.HasPrefix(digits, "0"):
		return digits[1:], nil
	default:
		return "", fmt.Errorf("invalid mobile for Vapio SMS")
	}
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
