package pinnacleSmsPayload

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	models "github.com/wecredit/communication-sdk/sdk/models"
)

const (
	MessageTypeCLICK = "CLICK"
	// DefaultSender is used only when TemplateHeader and TimesSmsApiSender are empty.
	// Production WeCredit Branch templates use WECRLP.
	DefaultSender = "WECRLP"
)

// GetTemplatePayload builds the Pinnacle Console JSON SMS body (no secrets).
// {#var#} substitution is applied once in sms.SendSmsByProcess via templatevars
// before any vendor call. Payload builders must not re-apply.
func GetTemplatePayload(data extapimodels.SmsRequestBody, cfg models.Config) (map[string]interface{}, error) {
	sender := strings.TrimSpace(data.TemplateHeader)
	if sender == "" {
		sender = strings.TrimSpace(cfg.TimesSmsApiSender)
	}
	if sender == "" {
		sender = DefaultSender
	}

	message := strings.TrimSpace(data.TemplateText)
	if message == "" {
		message = strings.TrimSpace(data.Description)
	}
	if message == "" {
		return nil, fmt.Errorf("Pinnacle SMS message text is empty")
	}

	entityID := strings.TrimSpace(data.TemplateEntityId)
	if entityID == "" {
		entityID = strings.TrimSpace(cfg.PinnacleSmsDltEntityId)
	}

	return BuildConsoleJSONPayload(data, sender, message, entityID)
}

// BuildConsoleJSONPayload builds the Console SMS JSON body for the given resolved fields.
func BuildConsoleJSONPayload(data extapimodels.SmsRequestBody, sender, message, entityID string) (map[string]interface{}, error) {
	number, err := NormalizeMSISDN(data.Mobile)
	if err != nil {
		return nil, err
	}

	msgItem := map[string]interface{}{
		"number": number,
		"text":   message,
	}
	if uid := sanitizeClientUID(data.CommId); uid != "" {
		msgItem["clientuid"] = uid
	}

	payload := map[string]interface{}{
		"sender":      strings.TrimSpace(sender),
		"message":     []map[string]interface{}{msgItem},
		"messagetype": chooseMessageType(message),
	}
	if data.DltTemplateId != 0 {
		payload["dlttempid"] = strconv.FormatInt(data.DltTemplateId, 10)
	}
	if strings.TrimSpace(entityID) != "" {
		payload["dltentityid"] = strings.TrimSpace(entityID)
	}
	return payload, nil
}

func chooseMessageType(message string) string {
	// CLICK enables Pinnacle's vendor URL shortening for English SMS content.
	// DLT URL whitelist (5901 URL_NOT_FOUND) remains a registration concern.
	_ = message
	return MessageTypeCLICK
}

// NormalizeMSISDN returns digits in 91XXXXXXXXXX form when possible.
func NormalizeMSISDN(mobile string) (string, error) {
	digits := onlyDigits(mobile)
	switch {
	case len(digits) == 10:
		return "91" + digits, nil
	case len(digits) == 12 && strings.HasPrefix(digits, "91"):
		return digits, nil
	case len(digits) == 11 && strings.HasPrefix(digits, "0"):
		return "91" + digits[1:], nil
	case len(digits) >= 10 && len(digits) <= 15:
		return digits, nil
	default:
		return "", fmt.Errorf("invalid mobile for Pinnacle SMS")
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

func sanitizeClientUID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}
