package templateCategorySync

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const categorySyncBatchSize = 100

// TemplateCategoryRow is one vendor API template entry (name + Meta fields).
type TemplateCategoryRow struct {
	Name     string
	Category string
	Status   string
}

// PayloadGroupKey returns a stable grouping key for batch SET payloads.
func PayloadGroupKey(computed ApplyCategoryUpdateResult, syncRunAt time.Time) string {
	touch := "nil"
	if computed.TouchCategoryOn {
		touch = syncRunAt.UTC().Format(time.RFC3339Nano)
	}

	return fmt.Sprintf("%s|%t|%s|%t|%s",
		computed.ProviderCategory,
		computed.IsActive,
		computed.Error,
		computed.TouchCategoryOn,
		touch,
	)
}

// GroupPendingByPayload groups deduped pending template names by identical update payload.
func GroupPendingByPayload(pending map[string]TemplateCategoryRow, syncRunAt time.Time) map[string][]string {
	groups := make(map[string][]string)
	for name, row := range pending {
		computed := ComputeCategoryUpdate(row.Name, row.Category, row.Status)
		key := PayloadGroupKey(computed, syncRunAt)
		groups[key] = append(groups[key], name)
	}
	for k := range groups {
		sort.Strings(groups[k])
	}

	return groups
}

// ChunkStrings splits names into chunks of at most size (size must be > 0).
func ChunkStrings(names []string, size int) [][]string {
	if size <= 0 || len(names) == 0 {
		return nil
	}

	out := make([][]string, 0, (len(names)+size-1)/size)
	for i := 0; i < len(names); i += size {
		end := i + size
		if end > len(names) {
			end = len(names)
		}
		out = append(out, names[i:end])
	}

	return out
}

// MergeTemplatesIntoPending records API templates in traversal order (last occurrence wins per name).
// Returns apiSeen (non-empty names from API) and skippedMissing (not in local set).
func MergeTemplatesIntoPending(
	local map[string]struct{},
	pending map[string]TemplateCategoryRow,
	rows []TemplateCategoryRow,
) (apiSeen, skippedMissing int) {
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			continue
		}

		apiSeen++
		if _, ok := local[name]; !ok {
			skippedMissing++
			continue
		}

		pending[name] = TemplateCategoryRow{
			Name:     name,
			Category: row.Category,
			Status:   row.Status,
		}
	}

	return apiSeen, skippedMissing
}

// ResolvePendingUpdate returns the effective category update for a name after ordered API merges.
func ResolvePendingUpdate(pending map[string]TemplateCategoryRow, name string) ApplyCategoryUpdateResult {
	row, ok := pending[name]
	if !ok {
		return ApplyCategoryUpdateResult{}
	}

	return ComputeCategoryUpdate(row.Name, row.Category, row.Status)
}

// MissingTemplateNames returns local template names that were not present in
// any successful provider listing. Callers should only use this after every
// configured panel for the vendor has completed successfully; a failed panel
// makes absence ambiguous.
func MissingTemplateNames(local map[string]struct{}, pending map[string]TemplateCategoryRow) []string {
	names := make([]string, 0)
	for name := range local {
		if _, ok := pending[name]; !ok {
			names = append(names, name)
		}
	}
	
	sort.Strings(names)
	return names
}
