package cache

import (
	"errors"
	"fmt"
	"math"
	"sort"
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
	activeRows := make([]apiModels.Templatedetails, 0, len(rows))

	for _, row := range rows {
		if row.Id <= 0 {
			return nil, fmt.Errorf("template snapshot contains invalid Id %d", row.Id)
		}
		// Runtime resolution can now retain multiple active rows for a resolution
		// tuple; include Id in every cache key so none overwrite another.
		if !row.IsActive {
			continue
		}
		activeRows = append(activeRows, row)
	}

	sort.Slice(activeRows, func(i, j int) bool { return activeRows[i].Id < activeRows[j].Id })
	for _, row := range activeRows {

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

		cacheKey := fmt.Sprintf("Process:%s|Stage:%s|Client:%s|Channel:%s|Vendor:%s", process, stage, client, channel, vendor)
		if _, exists := templates[cacheKey]; exists {
			cacheKey += fmt.Sprintf("|Id:%d", row.Id)
		}

		if _, exists := templates[cacheKey]; exists {
			return nil, fmt.Errorf("duplicate template cache key %q", cacheKey)
		}

		templates[cacheKey] = templateCacheData(row)

		_, reason := activeTemplateResolutionKey(row, stage, client, channel, vendor)
		if reason != "" {
			findings = append(findings, TemplateFinding{TemplateID: row.Id, Channel: channel, Reason: reason})
		}
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

	case "WHATSAPP":
		name := strings.TrimSpace(row.TemplateName)
		if name == "" {
			return "", "reference template has no TemplateName"
		}
		// AppId is part of WA active identity so the same TemplateName can exist
		// under multiple WABA apps (throughput). Empty AppId still participates.
		appID := strings.ToLower(strings.TrimSpace(row.AppId))
		return "named\x00" + process + "\x00" + strings.ToLower(name) + "\x00" + appID + "\x00" + base, ""

	case "RCS", "EMAIL", "PUSH", "IVR":
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
		"Id":                       int64(row.Id),
		"Client":                   row.Client,
		"Channel":                  row.Channel,
		"Process":                  row.Process,
		"Stage":                    stage,
		"Vendor":                   row.Vendor,
		"TemplateName":             row.TemplateName,
		"VoiceId":                  row.VoiceId,
		"ImageId":                  row.ImageId,
		"ImageUrl":                 row.ImageUrl,
		"DltTemplateId":            row.DltTemplateId,
		"TemplateEntityId":         row.TemplateEntityId,
		"TemplateHeader":           row.TemplateHeader,
		"IsActive":                 isActive,
		"TemplateText":             row.TemplateText,
		"Link":                     row.Link,
		"CreatedOn":                row.CreatedOn,
		"UpdatedOn":                row.UpdatedOn,
		"TemplateCategory":         row.TemplateCategory,
		"TemplateVariables":        row.TemplateVariables,
		"SmsFallbackVariables":     row.SmsFallbackVariables,
		"Subject":                  row.Subject,
		"FromEmail":                row.FromEmail,
		"AppId":                    row.AppId,
		"ProviderTemplateCategory": row.ProviderTemplateCategory,
		"LanguageCode":             row.LanguageCode,
		"CampaignId":               row.CampaignId,
		"CtaId":                    row.CtaId,
		"WabaNumber":               row.WabaNumber,
		"Error":                    row.Error,
		"CategoryUpdatedOn":        row.CategoryUpdatedOn,
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
