package templatevars

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

var varPlaceholder = regexp.MustCompile(`\{#var#\}`)
var bracePlaceholder = regexp.MustCompile(`\{#[^{}]*#\}`)
var namedPlaceholder = regexp.MustCompile(`(?i)#\s*([A-Za-z0-9_]+)\s*#`)

var ErrMixedTemplateFormat = errors.New("template cannot mix legacy and named placeholders")

type TemplateFormat string

const (
	TemplateFormatNone   TemplateFormat = ""
	TemplateFormatLegacy TemplateFormat = "legacy"
	TemplateFormatNamed  TemplateFormat = "named"
)

type NamedPlaceholder struct {
	Original string
	Name     string
}

var namedVariableFields = map[string]string{
	"LINK":              "PaymentLink",
	"NAME":              "CustomerName",
	"AMOUNT":            "EmiAmount",
	"DUEDATE":           "DueDate",
	"LOANID":            "LoanId",
	"APPLICATIONNUMBER": "ApplicationNumber",
}

func NormalizeNamedVariable(name string) string {
	return strings.ToUpper(strings.TrimSpace(name))
}

func IsSupportedNamedVariable(name string) bool {
	_, ok := namedVariableFields[NormalizeNamedVariable(name)]
	return ok
}

func ClassifyTemplateFormat(text string) (TemplateFormat, []string, []NamedPlaceholder, error) {
	legacyMatches := varPlaceholder.FindAllStringIndex(text, -1)
	masked := []byte(text)
	for _, match := range bracePlaceholder.FindAllStringIndex(text, -1) {
		for i := match[0]; i < match[1]; i++ {
			masked[i] = ' '
		}
	}

	namedMatches := namedPlaceholder.FindAllStringSubmatchIndex(string(masked), -1)
	named := make([]NamedPlaceholder, 0, len(namedMatches))
	for _, match := range namedMatches {
		original := text[match[0]:match[1]]
		captured := text[match[2]:match[3]]
		named = append(named, NamedPlaceholder{Original: original, Name: NormalizeNamedVariable(captured)})
	}

	switch {
	case len(legacyMatches) > 0 && len(named) > 0:
		return TemplateFormatNone, nil, nil, ErrMixedTemplateFormat
	case len(legacyMatches) > 0:
		matches := make([]string, len(legacyMatches))
		for i, match := range legacyMatches {
			matches[i] = text[match[0]:match[1]]
		}
		return TemplateFormatLegacy, matches, nil, nil
	case len(named) > 0:
		return TemplateFormatNamed, nil, named, nil
	default:
		return TemplateFormatNone, nil, nil, nil
	}
}

func ApplyNamedTemplateVariables(data extapimodels.SmsRequestBody) (string, error) {
	format, _, placeholders, err := ClassifyTemplateFormat(data.TemplateText)
	if err != nil {
		return "", err
	}

	if format != TemplateFormatNamed {
		return data.TemplateText, nil
	}
	for _, placeholder := range placeholders {
		if !IsSupportedNamedVariable(placeholder.Name) {
			return "", fmt.Errorf("unsupported named template variable %q; if this was not intended as a variable, remove the surrounding \"#\"", placeholder.Name)
		}
	}
	values := splitCSV(data.TemplateVariableValues)
	if len(values) == 0 {
		return "", fmt.Errorf("named template requires TemplateVariableValues for %d placeholders", len(placeholders))
	}
	if len(values) != len(placeholders) {
		return "", fmt.Errorf("named template has %d placeholders but TemplateVariableValues contains %d values", len(placeholders), len(values))
	}

	var replacementErr error
	placeholderIndex := 0
	text := namedPlaceholder.ReplaceAllStringFunc(data.TemplateText, func(match string) string {
		if replacementErr != nil {
			return ""
		}
		if placeholderIndex >= len(placeholders) {
			replacementErr = fmt.Errorf("named template placeholder/value sequence is inconsistent")
			return ""
		}
		placeholder := placeholders[placeholderIndex]
		value := values[placeholderIndex]
		placeholderIndex++
		if strings.TrimSpace(value) == "" {
			replacementErr = fmt.Errorf("missing value for named template variable %q", placeholder.Name)
			return ""
		}
		return value
	})

	if replacementErr != nil {
		return "", replacementErr
	}

	return text, nil
}

func resolveNamedVariable(name string, data extapimodels.SmsRequestBody) (string, bool, error) {
	name = NormalizeNamedVariable(name)
	if !IsSupportedNamedVariable(name) {
		return "", false, fmt.Errorf("unsupported named template variable %q; if this was not intended as a variable, remove the surrounding \"#\"", name)
	}
	values := map[string]string{
		"LINK": data.PaymentLink, "NAME": data.CustomerName, "AMOUNT": data.EmiAmount,
		"LOANID": data.LoanId, "APPLICATIONNUMBER": data.ApplicationNumber,
	}

	if name == "DUEDATE" {
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, data.DueDate); err == nil {
				return t.Format("2006-01-02"), true, nil
			}
		}

		return "", false, fmt.Errorf("invalid DueDate format: %s", data.DueDate)
	}

	return values[name], true, nil
}

// ApplyTemplateVariables replaces ordered {#var#} placeholders using TemplateVariables
// names and values from the SMS request (named payload fields and/or TemplateVariableValues CSV).
// Vendor-agnostic: Sinch, Pinnacle, Times, and any future SMS provider share this path.
func ApplyTemplateVariables(data extapimodels.SmsRequestBody) (string, error) {
	if !strings.Contains(data.TemplateText, "{#var#}") {
		return data.TemplateText, nil
	}

	extras, err := overlayPositionalTemplateValues(&data)
	if err != nil {
		return "", err
	}
	keys := splitCSV(data.TemplateVariables)
	variableMap := map[string]string{
		"EmiAmount":         data.EmiAmount,
		"ApplicationNumber": data.ApplicationNumber,
		"CustomerName":      data.CustomerName,
		"LoanId":            data.LoanId,
		"PaymentLink":       data.PaymentLink,
		"Link":              data.PaymentLink,
		"Description":       data.Description,
	}
	for key, value := range extras {
		variableMap[key] = value
	}

	keyIndex := 0
	var replacementErr error

	text := varPlaceholder.ReplaceAllStringFunc(data.TemplateText, func(_ string) string {
		if replacementErr != nil {
			return ""
		}
		if keyIndex >= len(keys) {
			replacementErr = fmt.Errorf("template has more {#var#} placeholders than TemplateVariables entries")
			return ""
		}

		key := strings.TrimSpace(keys[keyIndex])
		keyIndex++

		switch key {
		case "CustomerName":
			textValue := data.CustomerName
			if textValue == "" {
				textValue = "Dear Customer"
			}
			return textValue

		case "DueDate":
			if _, ok := variableMap["DueDate"]; !ok {
				dueDateStr := data.DueDate
				var formatted string
				var parsed bool

				layouts := []string{
					time.RFC3339,
					"2006-01-02 15:04:05 -0700 MST",
					"2006-01-02 15:04:05",
					"2006-01-02",
				}

				for _, layout := range layouts {
					if t, err := time.Parse(layout, dueDateStr); err == nil {
						formatted = t.Format("2006-01-02")
						parsed = true
						break
					}
				}

				if !parsed || strings.TrimSpace(formatted) == "" {
					replacementErr = fmt.Errorf("invalid DueDate format: %s", dueDateStr)
					return ""
				}

				variableMap["DueDate"] = formatted
			}

			return variableMap["DueDate"]

		case "EmiAmount":
			value := strings.TrimSpace(variableMap["EmiAmount"])
			if value == "" || value == "0" || value == "0.0" {
				replacementErr = fmt.Errorf("missing value for required variable: %s", key)
				return ""
			}
			return value

		case "PaymentLink", "Link":
			value := strings.TrimSpace(variableMap["PaymentLink"])
			if value == "" {
				replacementErr = fmt.Errorf("missing value for required variable: %s", key)
				return ""
			}
			return value

		default:
			// Dynamic DLT slots (urg, args, ...) may be intentionally blank.
			return lookupTemplateVar(variableMap, key)
		}
	})

	if replacementErr != nil {
		return "", replacementErr
	}
	return text, nil
}

func lookupTemplateVar(variableMap map[string]string, key string) string {
	if value, ok := variableMap[key]; ok {
		return value
	}
	for mapKey, mapValue := range variableMap {
		if strings.EqualFold(mapKey, key) {
			return mapValue
		}
	}
	return ""
}

// overlayPositionalTemplateValues fills named fields from CommMarketingInput.VariablesValue.
// ZapCash/legacy already set CustomerName/PaymentLink/etc and leave TemplateVariableValues empty.
// Unknown names (urg, args, ...) stay in extras and fill {#var#} in TemplateVariables order.
func overlayPositionalTemplateValues(data *extapimodels.SmsRequestBody) (map[string]string, error) {
	extras := map[string]string{}
	if data == nil || strings.TrimSpace(data.TemplateVariableValues) == "" {
		return extras, nil
	}
	keys := splitCSV(data.TemplateVariables)
	values := splitCSV(data.TemplateVariableValues)
	if len(keys) != len(values) {
		return nil, fmt.Errorf("template variable count does not match supplied values")
	}
	for i, key := range keys {
		value := values[i]
		switch strings.ToLower(key) {
		case "customername":
			data.CustomerName = value
		case "emiamount":
			data.EmiAmount = value
		case "loanid":
			data.LoanId = value
		case "applicationnumber":
			data.ApplicationNumber = value
		case "duedate":
			data.DueDate = value
		case "description":
			data.Description = value
		case "paymentlink", "link":
			data.PaymentLink = value
		default:
			extras[key] = value
		}
	}
	return extras, nil
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
