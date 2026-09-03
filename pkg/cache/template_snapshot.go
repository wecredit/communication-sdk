package cache

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
)

type TemplateFinding struct {
	TemplateID int
	Channel    string
	Reason     string
}

type TemplateSnapshot struct {
	Templates map[string]map[string]interface{}
	Findings  []TemplateFinding
}

var installedTemplateSnapshot atomic.Pointer[TemplateSnapshot]

// BuildTemplateSnapshot prepares and validates a complete replacement cache.
// The caller installs it only after the whole build succeeds, so readers never
// observe a partially rebuilt cache and a failed reload keeps the previous one.
func BuildTemplateSnapshot(rows []apiModels.Templatedetails) (*TemplateSnapshot, error) {
	templates := make(map[string]map[string]interface{}, len(rows))
	findings := make([]TemplateFinding, 0)
	activeKeys := make(map[string]int)

	for _, row := range rows {
		if row.Id <= 0 {
			return nil, fmt.Errorf("template snapshot contains invalid Id %d", row.Id)
		}
		// The runtime cache resolves only active templates. Inactive historical or
		// alternate rows may legitimately share a stage resolution tuple and must
		// not block startup or compete with the active row.
		if !row.IsActive {
			continue
		}

		process := strings.TrimSpace(row.Process)
		client := strings.TrimSpace(row.Client)
		channel := strings.ToUpper(strings.TrimSpace(row.Channel))
		vendor := strings.ToUpper(strings.TrimSpace(row.Vendor))

		stage := ""
		if row.Stage != nil {
			var err error
			stage, err = CanonicalTemplateStage(*row.Stage)
			if err != nil {
				return nil, fmt.Errorf("template %d: %w", row.Id, err)
			}
		}

		cacheKey := fmt.Sprintf("Process:%s|Stage:<nil>|Id:%d|Client:%s|Channel:%s|Vendor:%s", process, row.Id, client, channel, vendor)
		if row.Stage != nil {
			cacheKey = fmt.Sprintf("Process:%s|Stage:%s|Client:%s|Channel:%s|Vendor:%s", process, stage, client, channel, vendor)
		}

		if _, exists := templates[cacheKey]; exists {
			return nil, fmt.Errorf("duplicate template cache key %q", cacheKey)
		}

		templates[cacheKey] = templateCacheData(row)

		resolutionKey, reason := activeTemplateResolutionKey(row, stage, client, channel, vendor)
		if reason != "" {
			findings = append(findings, TemplateFinding{TemplateID: row.Id, Channel: channel, Reason: reason})
			continue
		}

		if existingID, exists := activeKeys[resolutionKey]; exists {
			return nil, fmt.Errorf("active templates %d and %d share resolution key %q", existingID, row.Id, resolutionKey)
		}

		activeKeys[resolutionKey] = row.Id
	}

	return &TemplateSnapshot{Templates: templates, Findings: findings}, nil
}

// InstallTemplateSnapshot installs the template snapshot
func InstallTemplateSnapshot(snapshot *TemplateSnapshot) error {
	if snapshot == nil || snapshot.Templates == nil {
		return errors.New("complete template snapshot is required")
	}

	installedTemplateSnapshot.Store(snapshot)
	return nil
}

// CurrentTemplateSnapshot returns the current template snapshot
func CurrentTemplateSnapshot() (*TemplateSnapshot, bool) {
	snapshot := installedTemplateSnapshot.Load()
	return snapshot, snapshot != nil
}

// activeTemplateResolutionKey returns the active template resolution key
func activeTemplateResolutionKey(row apiModels.Templatedetails, stage, client, channel, vendor string) (string, string) {
	base := strings.Join([]string{strings.ToLower(client), channel, vendor}, "\x00")
	process := strings.ToLower(strings.TrimSpace(row.Process))
	if row.Stage != nil {
		return "stage\x00" + process + "\x00" + stage + "\x00" + base, ""
	}

	switch channel {
	case "SMS":
		if row.DltTemplateId <= 0 {
			return "", "reference SMS template has no DltTemplateId"
		}
		return fmt.Sprintf("sms\x00%s\x00%d\x00%s", process, row.DltTemplateId, base), ""

	case "RCS", "WHATSAPP", "EMAIL", "PUSH":
		name := strings.TrimSpace(row.TemplateName)
		if name == "" {
			return "", "reference template has no TemplateName"
		}
		return "named\x00" + process + "\x00" + strings.ToLower(name) + "\x00" + base, ""

	default:
		return "", "template channel is unsupported"
	}
}

// templateCacheData preserves the established runtime cache value types while
// the database load itself remains strongly typed.
func templateCacheData(row apiModels.Templatedetails) map[string]interface{} {
	isActive := int64(0)
	if row.IsActive {
		isActive = 1
	}
	var stage interface{}
	if row.Stage != nil {
		stage = *row.Stage
	}

	return map[string]interface{}{
		"Id":                   int64(row.Id),
		"Client":               row.Client,
		"Channel":              row.Channel,
		"Process":              row.Process,
		"Stage":                stage,
		"Vendor":               row.Vendor,
		"TemplateName":         row.TemplateName,
		"ImageId":              row.ImageId,
		"ImageUrl":             row.ImageUrl,
		"DltTemplateId":        row.DltTemplateId,
		"TemplateEntityId":     row.TemplateEntityId,
		"TemplateHeader":       row.TemplateHeader,
		"IsActive":             isActive,
		"TemplateText":         row.TemplateText,
		"Link":                 row.Link,
		"CreatedOn":            row.CreatedOn,
		"UpdatedOn":            row.UpdatedOn,
		"TemplateCategory":     row.TemplateCategory,
		"TemplateVariables":    row.TemplateVariables,
		"SmsFallbackVariables": row.SmsFallbackVariables,
		"Subject":              row.Subject,
		"FromEmail":            row.FromEmail,
	}
}

// CanonicalTemplateStage converts the API/runtime float representation once
// and uses scaled integer hundredths for cache identity and comparisons.
func CanonicalTemplateStage(stage float64) (string, error) {
	if math.IsNaN(stage) || math.IsInf(stage, 0) {
		return "", errors.New("Stage must be finite")
	}

	scaledValue := stage * 100
	hundredths := int64(math.Round(scaledValue))
	if math.Abs(scaledValue-float64(hundredths)) > 0.0000001 {
		return "", fmt.Errorf("Stage %.10g has more than two decimal places", stage)
	}

	sign := ""
	if hundredths < 0 {
		sign = "-"
		hundredths = -hundredths
	}

	return fmt.Sprintf("%s%d.%02d", sign, hundredths/100, hundredths%100), nil
}
