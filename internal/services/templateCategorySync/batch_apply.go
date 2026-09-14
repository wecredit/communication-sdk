package templateCategorySync

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// Batch failure policy: log + metric, one chunk retry, split-in-half retry, then per-name
// applyCategoryUpdate (time.Now() for CategoryUpdatedOn). Cron self-heals on next tick.

// flushCategoryUpdates applies batched UPDATEs for all pending names for one vendor.
func flushCategoryUpdates(vendor string, pending map[string]TemplateCategoryRow, syncRunAt time.Time) (int, error) {
	if len(pending) == 0 {
		return 0, nil
	}

	groupKeys := GroupPendingByPayload(pending, syncRunAt)
	keys := make([]string, 0, len(groupKeys))
	for k := range groupKeys {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var total int
	for _, key := range keys {
		names := groupKeys[key]
		if len(names) == 0 {
			continue
		}

		sample := pending[names[0]]
		computed := ComputeCategoryUpdate(sample.Name, sample.Category, sample.Status)
		n, err := applyNamesInBatches(vendor, names, computed, syncRunAt, pending)

		if err != nil {
			return total, err
		}

		total += n
	}

	return total, nil
}

func applyNamesInBatches(vendor string, names []string, computed ApplyCategoryUpdateResult, syncRunAt time.Time, pending map[string]TemplateCategoryRow) (int, error) {
	var total int
	for _, chunk := range ChunkStrings(names, categorySyncBatchSize) {
		n, err := applyChunkWithFailurePolicy(vendor, chunk, computed, syncRunAt, pending)
		if err != nil {
			return total, err
		}
		total += n
	}

	return total, nil
}

// applyChunkWithFailurePolicy: retry batch once, split halves on repeat failure, per-name fallback uses time.Now().
func applyChunkWithFailurePolicy(vendor string, names []string, computed ApplyCategoryUpdateResult, syncRunAt time.Time, pending map[string]TemplateCategoryRow) (int, error) {
	if len(names) == 0 {
		return 0, nil
	}

	n, err := applyBatchCategoryUpdate(vendor, names, computed, syncRunAt)
	if err == nil {
		return n, nil
	}

	utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_batch_failed_total vendor=%s chunk_size=%d: %v", vendor, len(names), err))

	n, err = applyBatchCategoryUpdate(vendor, names, computed, syncRunAt)
	if err == nil {
		return n, nil
	}
	utils.Error(fmt.Errorf("metric_name=whatsapp_template_sync_batch_failed_total vendor=%s chunk_size=%d retry: %v", vendor, len(names), err))

	if len(names) == 1 {
		row := pending[names[0]]
		return applyCategoryUpdate(vendor, row.Name, row.Category, row.Status)
	}

	mid := len(names) / 2
	n1, _ := applyChunkWithFailurePolicy(vendor, names[:mid], computed, syncRunAt, pending)
	n2, _ := applyChunkWithFailurePolicy(vendor, names[mid:], computed, syncRunAt, pending)

	return n1 + n2, nil
}

func applyBatchCategoryUpdate(vendor string, names []string, computed ApplyCategoryUpdateResult, syncRunAt time.Time) (int, error) {
	var touchedAt *time.Time
	if computed.TouchCategoryOn {
		t := syncRunAt
		touchedAt = &t
	}

	updates := categoryUpdatesMap(computed, touchedAt)
	res := database.DBtechWrite.Table(config.Configs.TemplateDetailsTable).
		Where("Channel = ? AND Vendor = ? AND TemplateName IN ?", variables.WhatsApp, vendor, names).
		Updates(updates)
	if res.Error != nil {
		return 0, res.Error
	}

	return int(res.RowsAffected), nil
}

func categoryUpdatesMap(computed ApplyCategoryUpdateResult, categoryUpdatedOn *time.Time) map[string]interface{} {
	updates := map[string]interface{}{
		"ProviderTemplateCategory": computed.ProviderCategory,
		"IsActive":                 computed.IsActive,
		"Error":                    computed.Error,
	}

	if computed.TouchCategoryOn {
		updates["CategoryUpdatedOn"] = categoryUpdatedOn
	} else {
		updates["CategoryUpdatedOn"] = nil
	}

	return updates
}

func loadLocalTemplateNames(vendor string) (map[string]struct{}, error) {
	var names []string
	err := database.DBtechWrite.Table(config.Configs.TemplateDetailsTable).
		Where("Channel = ? AND Vendor = ? AND TemplateName IS NOT NULL AND TemplateName <> ''", variables.WhatsApp, vendor).
		Distinct("TemplateName").
		Pluck("TemplateName", &names).Error
	if err != nil {
		return nil, err
	}

	out := make(map[string]struct{}, len(names))
	for _, raw := range names {
		n := strings.TrimSpace(raw)
		if n == "" {
			continue
		}
		out[n] = struct{}{}
	}

	return out, nil
}
