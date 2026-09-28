package whatsappPayload

import (
	"fmt"
	"strings"
	"time"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// ButtonMobileSubstitute returns DynamicMobile when set, else recipient Mobile.
func ButtonMobileSubstitute(req extapimodels.WhatsappRequestBody) string {
	if strings.TrimSpace(req.DynamicMobile) != "" {
		return req.DynamicMobile
	}
	return req.Mobile
}

// SubstituteButtonLinkMobile replaces "<mobile>" in buttonLink with the CTA
// substitute (hermis parity: plain replace, no process-specific digit rotation).
func SubstituteButtonLinkMobile(buttonLink, mobileSub string) string {
	if strings.Contains(buttonLink, "<mobile>") {
		return strings.Replace(buttonLink, "<mobile>", mobileSub, 1)
	}
	return buttonLink
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

// NamedBodyParams builds WhatsApp body text params from TemplateVariables CSV
// keys (CustomerName, DueDate, …) when positional TemplateVariableValues is empty.
func NamedBodyParams(req extapimodels.WhatsappRequestBody) []map[string]interface{} {
	keysCSV := strings.TrimSpace(req.TemplateVariables)
	if keysCSV == "" {
		return nil
	}
	keys := strings.Split(keysCSV, ",")
	out := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		textValue := namedTemplateVarValue(req, key)
		if textValue == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"type": "text",
			"text": textValue,
		})
	}
	return out
}

// BodyParams prefers positional TemplateVariableValues; falls back to named TemplateVariables.
func BodyParams(req extapimodels.WhatsappRequestBody) []map[string]interface{} {
	if positional := PositionalBodyParams(req.TemplateVariableValues); len(positional) > 0 {
		return positional
	}
	return NamedBodyParams(req)
}

func namedTemplateVarValue(req extapimodels.WhatsappRequestBody, key string) string {
	switch key {
	case "CustomerName":
		if req.CustomerName == "" {
			return "Customer"
		}
		return req.CustomerName
	case "DueDate":
		return formatDueDate(req.DueDate)
	case "LoanId":
		return req.LoanId
	case "ApplicationNumber":
		return req.ApplicationNumber
	case "EmiAmount":
		return req.EmiAmount
	default:
		return ""
	}
}

func formatDueDate(dueDateStr string) string {
	formatted := dueDateStr
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, dueDateStr); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if dueDateStr != "" {
		utils.Error(fmt.Errorf("invalid DueDate format: %s", dueDateStr))
	}
	return formatted
}
