package templateCategorySync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/configurationcache"
	"github.com/wecredit/communication-sdk/internal/database"
	internalredis "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

const (
	timesTemplateListPathDefault     = "/wa/v1/templates/get-list"
	AppConfigKeyWhatsappVendorBaseURLs = "WHATSAPP_VENDOR_BASE_URLS"
)

// vendorPanel is one Times/Pinnacle entry from AppConfig WHATSAPP_VENDOR_BASE_URLS.
// ApiID is credential-class — never log it.
type vendorPanel struct {
	ApiSource string `json:"api_source"`
	BaseURL   string `json:"base_url"`
	ApiID     string `json:"api_id"`
}

// SyncTimesAndPinnacle refreshes ProviderTemplateCategory / IsActive on WHATSAPP
// TemplateDetails rows for TIMES and PINNACLE vendors (hermis category checkers ported).
//
// Panel hosts come from Communication MySQL AppConfig key WHATSAPP_VENDOR_BASE_URLS
// (reloaded every run). Gated by WHATSAPP_TEMPLATE_SYNC_ENABLED=true.
func SyncTimesAndPinnacle() error {
	if database.DBtechWrite == nil {
		return fmt.Errorf("tech write DB not initialized")
	}
	enabled := strings.EqualFold(strings.TrimSpace(config.Configs.WhatsappTemplateSyncEnabled), "true")
	if !enabled {
		utils.Info("WhatsApp template category sync disabled (set WHATSAPP_TEMPLATE_SYNC_ENABLED=true)")
		return nil
	}

	var updated int
	var runErrs []string

	if n, err := syncTimes(); err != nil {
		utils.Error(fmt.Errorf("Times template category sync: %v", err))
		runErrs = append(runErrs, err.Error())
	} else {
		updated += n
	}
	if n, err := syncPinnacle(); err != nil {
		utils.Error(fmt.Errorf("Pinnacle template category sync: %v", err))
		runErrs = append(runErrs, err.Error())
	} else {
		updated += n
	}

	utils.Info(fmt.Sprintf("WhatsApp template category sync finished: rows_touched=%d", updated))
	if updated > 0 {
		publishTemplateCacheInvalidation()
	}
	if len(runErrs) > 0 {
		return fmt.Errorf("WhatsApp template category sync incomplete: %s", strings.Join(runErrs, "; "))
	}
	return nil
}

func syncTimes() (int, error) {
	panels, err := loadVendorPanelsForSource("times")
	if err != nil {
		return 0, err
	}
	if len(panels) == 0 {
		if allowSingleHostFallback() {
			utils.Info("Times template sync: AppConfig panels empty; using single-host fallback (UAT flag)")
			return syncTimesSingleHostFallback()
		}
		utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panels_empty vendor=times: no AppConfig panels"))
		return 0, fmt.Errorf("Times panels empty in AppConfig %s (set WHATSAPP_TEMPLATE_SYNC_ALLOW_SINGLE_HOST_FALLBACK=true only for UAT)", AppConfigKeyWhatsappVendorBaseURLs)
	}

	endpoint := strings.TrimSpace(config.Configs.TimesWpTemplateListEndpoint)
	if endpoint == "" {
		endpoint = timesTemplateListPathDefault
	}

	var total, failed int
	for _, panel := range panels {
		base := strings.TrimRight(strings.TrimSpace(panel.BaseURL), "/")
		if base == "" || strings.TrimSpace(panel.ApiID) == "" {
			utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panel_failed_total vendor=times base_url=%s: missing base_url or api_id", base))
			failed++
			continue
		}
		listURL := base + "/" + strings.TrimLeft(endpoint, "/")
		n, panelErr := syncTimesPanel(listURL, panel.ApiID, base)
		if panelErr != nil {
			utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panel_failed_total vendor=times base_url=%s: %v", base, panelErr))
			failed++
			continue
		}
		total += n
	}
	if failed > 0 {
		return total, fmt.Errorf("Times template sync: %d of %d panel(s) failed", failed, len(panels))
	}
	return total, nil
}

func syncTimesPanel(listURL, apiID, baseURLForLog string) (int, error) {
	headers := map[string]string{
		"Authorization": apiID,
		"Content-Type":  "application/json",
	}
	payload := map[string]interface{}{
		"page_number": "",
		"page_size":   "",
	}
	apiResponse, err := utils.ApiHit("POST", listURL, headers, "", "", payload, variables.ContentTypeJSON)
	if err != nil {
		return 0, err
	}
	if err := utils.ErrIfHTTPNotOK(apiResponse); err != nil {
		return 0, fmt.Errorf("Times template list: %w", err)
	}
	status, _ := apiResponse["status"].(bool)
	if !status {
		return 0, fmt.Errorf("list returned status=false")
	}
	resJSON, _ := apiResponse["res_json"].(map[string]interface{})
	templates, _ := resJSON["data"].([]interface{})
	var total int
	for _, item := range templates {
		tpl, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := tpl["name"].(string)
		category, _ := tpl["category"].(string)
		statusStr, _ := tpl["status"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		n, err := applyCategoryUpdate(variables.TIMES, name, category, statusStr)
		if err != nil {
			utils.Error(fmt.Errorf("Times update %s base_url=%s: %v", name, baseURLForLog, err))
			continue
		}
		total += n
	}
	return total, nil
}

func syncTimesSingleHostFallback() (int, error) {
	baseURL := timesTemplateListBaseURL()
	if baseURL == "" {
		return 0, fmt.Errorf("Times template list base URL not configured (TIMES_WP_TEMPLATE_LIST_BASE_URL or TIMES_WP_API_URL)")
	}
	endpoint := strings.TrimSpace(config.Configs.TimesWpTemplateListEndpoint)
	if endpoint == "" {
		endpoint = timesTemplateListPathDefault
	}
	listURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")

	appIDs, err := distinctAppIDs(variables.TIMES)
	if err != nil {
		return 0, err
	}
	if len(appIDs) == 0 {
		utils.Info("Times template sync fallback: no TemplateDetails AppId values")
		return 0, nil
	}

	var total, failed int
	for _, appID := range appIDs {
		n, panelErr := syncTimesPanel(listURL, appID, baseURL)
		if panelErr != nil {
			utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panel_failed_total vendor=times base_url=%s: %v", baseURL, panelErr))
			failed++
			continue
		}
		total += n
	}
	if failed > 0 {
		return total, fmt.Errorf("Times single-host fallback: %d of %d AppId(s) failed", failed, len(appIDs))
	}
	return total, nil
}

func syncPinnacle() (int, error) {
	apiKey := strings.TrimSpace(config.Configs.PinnacleWhatsappApiKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(config.Configs.PinnacleZapcashWhatsappApiKey)
	}
	if apiKey == "" {
		return 0, fmt.Errorf("Pinnacle API key not configured (PINNACLE_WP_API_KEY or PINNACLE_ZAPCASH_WHATSAPP_API_KEY)")
	}

	panels, err := loadVendorPanelsForSource("pinnacle")
	if err != nil {
		return 0, err
	}
	if len(panels) == 0 {
		if allowSingleHostFallback() {
			utils.Info("Pinnacle template sync: AppConfig panels empty; using TemplateDetails AppIds + list base (UAT flag)")
			return syncPinnacleSingleHostFallback(apiKey)
		}
		utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panels_empty vendor=pinnacle: no AppConfig panels"))
		return 0, fmt.Errorf("Pinnacle panels empty in AppConfig %s (set WHATSAPP_TEMPLATE_SYNC_ALLOW_SINGLE_HOST_FALLBACK=true only for UAT)", AppConfigKeyWhatsappVendorBaseURLs)
	}

	headers := map[string]string{
		"apikey":       apiKey,
		"Content-Type": "application/json",
	}

	var total, failed int
	for _, panel := range panels {
		base := strings.TrimRight(strings.TrimSpace(panel.BaseURL), "/")
		appID := strings.TrimSpace(panel.ApiID)
		if base == "" || appID == "" {
			utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panel_failed_total vendor=pinnacle base_url=%s: missing base_url or api_id", base))
			failed++
			continue
		}
		n, panelErr := syncPinnaclePanel(headers, base, appID)
		if panelErr != nil {
			utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panel_failed_total vendor=pinnacle base_url=%s: %v", base, panelErr))
			failed++
			continue
		}
		total += n
	}
	if failed > 0 {
		return total, fmt.Errorf("Pinnacle template sync: %d of %d panel(s) failed", failed, len(panels))
	}
	return total, nil
}

func syncPinnacleSingleHostFallback(apiKey string) (int, error) {
	baseURL := strings.TrimSpace(config.Configs.PinnacleWhatsappTemplateListBaseUrl)
	if baseURL == "" {
		baseURL = strings.TrimSpace(config.Configs.PinnacleWhatsappBaseUrl)
	}
	if baseURL == "" {
		return 0, fmt.Errorf("PINNACLE_WP_TEMPLATE_LIST_BASE_URL (or PINNACLE_WP_BASE_URL) not configured")
	}
	appIDs, err := distinctAppIDs(variables.PINNACLE)
	if err != nil {
		return 0, err
	}
	if len(appIDs) == 0 {
		utils.Info("Pinnacle template sync fallback: no TemplateDetails AppId values")
		return 0, nil
	}
	headers := map[string]string{
		"apikey":       apiKey,
		"Content-Type": "application/json",
	}
	base := strings.TrimRight(baseURL, "/")
	var total, failed int
	for _, appID := range appIDs {
		n, panelErr := syncPinnaclePanel(headers, base, appID)
		if panelErr != nil {
			utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_panel_failed_total vendor=pinnacle base_url=%s: %v", base, panelErr))
			failed++
			continue
		}
		total += n
	}
	if failed > 0 {
		return total, fmt.Errorf("Pinnacle single-host fallback: %d of %d AppId(s) failed", failed, len(appIDs))
	}
	return total, nil
}

func syncPinnaclePanel(headers map[string]string, baseURL, appID string) (int, error) {
	nextURL := strings.TrimRight(baseURL, "/") + "/" + strings.Trim(appID, "/") + "/message_templates"
	var total int
	gotPage := false
	for nextURL != "" {
		apiResponse, err := utils.ApiHit("GET", nextURL, headers, "", "", nil, variables.ContentTypeJSON)
		if err != nil {
			return total, err
		}
		if err := utils.ErrIfHTTPNotOK(apiResponse); err != nil {
			return total, fmt.Errorf("Pinnacle template list: %w", err)
		}
		gotPage = true
		page, _ := apiResponse["data"].([]interface{})
		for _, item := range page {
			tpl, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := tpl["name"].(string)
			category, _ := tpl["category"].(string)
			statusStr, _ := tpl["status"].(string)
			if strings.TrimSpace(name) == "" {
				continue
			}
			n, err := applyCategoryUpdate(variables.PINNACLE, name, category, statusStr)
			if err != nil {
				utils.Error(fmt.Errorf("Pinnacle update %s: %v", name, err))
				continue
			}
			total += n
		}
		paging, _ := apiResponse["paging"].(map[string]interface{})
		next, _ := paging["next"].(string)
		nextURL = strings.TrimSpace(next)
	}
	if !gotPage {
		return 0, fmt.Errorf("no response pages")
	}
	return total, nil
}

func allowSingleHostFallback() bool {
	return strings.EqualFold(strings.TrimSpace(config.Configs.WhatsappTemplateSyncAllowSingleHostFallback), "true")
}

// loadVendorPanelsForSource reloads AppConfig WHATSAPP_VENDOR_BASE_URLS every call.
func loadVendorPanelsForSource(apiSource string) ([]vendorPanel, error) {
	all, err := loadVendorPanels()
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(apiSource))
	out := make([]vendorPanel, 0, len(all))
	for _, p := range all {
		if strings.ToLower(strings.TrimSpace(p.ApiSource)) == want {
			out = append(out, p)
		}
	}
	return out, nil
}

func loadVendorPanels() ([]vendorPanel, error) {
	raw, err := fetchAppConfigValue(AppConfigKeyWhatsappVendorBaseURLs)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return ParseVendorPanelsJSON(raw)
}

// ParseVendorPanelsJSON parses AppConfig JSON (exported for tests). Never logs raw.
func ParseVendorPanelsJSON(raw string) ([]vendorPanel, error) {
	var panels []vendorPanel
	if err := json.Unmarshal([]byte(raw), &panels); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %w", AppConfigKeyWhatsappVendorBaseURLs, err)
	}
	return panels, nil
}

func fetchAppConfigValue(configKey string) (string, error) {
	db := database.DBtechWrite
	if db == nil {
		db = database.DBtechRead
	}
	if db == nil {
		return "", fmt.Errorf("communication database is not initialized")
	}
	table := strings.TrimSpace(config.Configs.AppConfigTableName)
	if table == "" {
		table = "AppConfig"
	}
	var value string
	query := fmt.Sprintf("SELECT ConfigValue FROM %s WHERE ConfigKey = ? LIMIT 1", table)
	err := db.Raw(query, configKey).Scan(&value).Error
	if err != nil {
		return "", fmt.Errorf("fetch AppConfig %s: %w", configKey, err)
	}
	return value, nil
}

func distinctAppIDs(vendor string) ([]string, error) {
	var appIDs []string
	err := database.DBtechWrite.Table(config.Configs.TemplateDetailsTable).
		Where("Channel = ? AND Vendor = ? AND AppId IS NOT NULL AND AppId <> ''", variables.WhatsApp, vendor).
		Distinct("AppId").
		Pluck("AppId", &appIDs).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(appIDs))
	seen := map[string]struct{}{}
	for _, raw := range appIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// ApplyCategoryUpdateResult is exported for unit tests of hermis parity rules.
type ApplyCategoryUpdateResult struct {
	ProviderCategory string
	IsActive         bool
	Error            string
	TouchCategoryOn  bool
}

// ComputeCategoryUpdate mirrors hermis update_template_utility_status semantics.
// UTILITY-named templates that Meta reclassifies away from UTILITY stay updated
// on ProviderTemplateCategory but are forced inactive so they are not dispatched.
func ComputeCategoryUpdate(templateName, apiCategory, apiStatus string) ApplyCategoryUpdateResult {
	category := strings.TrimSpace(apiCategory)
	status := strings.ToUpper(strings.TrimSpace(apiStatus))
	isActive := status == "APPROVED"
	errMsg := ""
	touch := false
	if strings.Contains(strings.ToLower(templateName), "utility") && !strings.EqualFold(category, "UTILITY") {
		errMsg = fmt.Sprintf("Template changes Utility to %s category", category)
		touch = true
		isActive = false
	}
	return ApplyCategoryUpdateResult{
		ProviderCategory: category,
		IsActive:         isActive,
		Error:            errMsg,
		TouchCategoryOn:  touch,
	}
}

func applyCategoryUpdate(vendor, templateName, apiCategory, apiStatus string) (int, error) {
	// Hermis updates by template_name only with no advisory/mutation lock against
	// admin template APIs — same last-writer-wins columns here.
	computed := ComputeCategoryUpdate(templateName, apiCategory, apiStatus)
	updates := map[string]interface{}{
		"ProviderTemplateCategory": computed.ProviderCategory,
		"IsActive":                 computed.IsActive,
		"Error":                    computed.Error,
	}
	if computed.TouchCategoryOn {
		now := time.Now()
		updates["CategoryUpdatedOn"] = now
	} else {
		// Clear stale mismatch error when category is healthy again.
		updates["CategoryUpdatedOn"] = nil
	}

	res := database.DBtechWrite.Table(config.Configs.TemplateDetailsTable).
		Where("Channel = ? AND Vendor = ? AND TemplateName = ?", variables.WhatsApp, vendor, templateName).
		Updates(updates)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

func timesTemplateListBaseURL() string {
	if v := strings.TrimSpace(config.Configs.TimesWpTemplateListBaseUrl); v != "" {
		return strings.TrimRight(v, "/")
	}
	raw := strings.TrimSpace(config.Configs.TimesWpApiUrl)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func publishTemplateCacheInvalidation() {
	if database.DBtechWrite == nil || strings.TrimSpace(config.Configs.ConfigurationVersionTable) == "" {
		return
	}
	version, err := configurationcache.IncrementTemplateVersion(database.DBtechWrite, config.Configs.ConfigurationVersionTable)
	if err != nil {
		utils.Error(fmt.Errorf("template category sync: increment version: %v", err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := configurationcache.PublishTemplateInvalidation(ctx, internalredis.RDB, config.Configs.Environment, version); err != nil {
		utils.Error(fmt.Errorf("template category sync: publish invalidation: %v", err))
		return
	}
	utils.Info(fmt.Sprintf("template category sync: cache invalidation published version=%d", version))
}
