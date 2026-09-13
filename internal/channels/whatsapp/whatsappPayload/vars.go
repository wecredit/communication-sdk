package whatsappPayload

import (
	"strings"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

// ButtonMobileSubstitute returns DynamicMobile when set, else recipient Mobile.
func ButtonMobileSubstitute(req extapimodels.WhatsappRequestBody) string {
	if strings.TrimSpace(req.DynamicMobile) != "" {
		return req.DynamicMobile
	}
	return req.Mobile
}

// PositionalBodyParams builds WhatsApp body text params from VariablesValue CSV
// (hermis variable1,variable2). Returns nil when CSV is empty.
func PositionalBodyParams(csv string) []map[string]interface{} {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]map[string]interface{}, 0, len(parts))
	for _, p := range parts {
		text := strings.TrimSpace(p)
		out = append(out, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}
	return out
}
