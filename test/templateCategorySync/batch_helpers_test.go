package templateCategorySync_test

import (
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/services/templateCategorySync"
)

func TestPayloadGroupKeySameSyncRunAtCoalesces(t *testing.T) {
	syncRunAt := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	a := templateCategorySync.ComputeCategoryUpdate("a_utility_v1", "MARKETING", "APPROVED")
	b := templateCategorySync.ComputeCategoryUpdate("b_utility_v1", "MARKETING", "APPROVED")
	if templateCategorySync.PayloadGroupKey(a, syncRunAt) != templateCategorySync.PayloadGroupKey(b, syncRunAt) {
		t.Fatal("same mismatch outcome + syncRunAt should share batch group key")
	}
	diff := templateCategorySync.ComputeCategoryUpdate("c_utility_v1", "UTILITY", "APPROVED")
	if templateCategorySync.PayloadGroupKey(a, syncRunAt) == templateCategorySync.PayloadGroupKey(diff, syncRunAt) {
		t.Fatal("different category should not share group key")
	}
}

func TestPayloadGroupKeyDiffersPerSyncRunAtWhenTouch(t *testing.T) {
	computed := templateCategorySync.ComputeCategoryUpdate("x_utility", "MARKETING", "APPROVED")
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Second)
	if templateCategorySync.PayloadGroupKey(computed, t1) == templateCategorySync.PayloadGroupKey(computed, t2) {
		t.Fatal("TouchCategoryOn groups should include syncRunAt in key")
	}
}

func TestChunkStrings(t *testing.T) {
	names := make([]string, 250)
	for i := range names {
		names[i] = "tpl"
	}
	chunks := templateCategorySync.ChunkStrings(names, 100)
	if len(chunks) != 3 {
		t.Fatalf("len(chunks)=%d want 3", len(chunks))
	}
	if len(chunks[0]) != 100 || len(chunks[1]) != 100 || len(chunks[2]) != 50 {
		t.Fatalf("chunk sizes = %d %d %d", len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
}

func TestMergeTemplatesIntoPendingLastWins(t *testing.T) {
	local := map[string]struct{}{"foo_utility_v1": {}}
	pending := make(map[string]templateCategorySync.TemplateCategoryRow)
	rows := []templateCategorySync.TemplateCategoryRow{
		{Name: "foo_utility_v1", Category: "UTILITY", Status: "APPROVED"},
		{Name: "foo_utility_v1", Category: "MARKETING", Status: "APPROVED"},
	}
	seen, skipped := templateCategorySync.MergeTemplatesIntoPending(local, pending, rows)
	if seen != 2 || skipped != 0 {
		t.Fatalf("seen=%d skipped=%d", seen, skipped)
	}
	got := templateCategorySync.ResolvePendingUpdate(pending, "foo_utility_v1")
	if got.ProviderCategory != "MARKETING" || !got.TouchCategoryOn {
		t.Fatalf("last wins = %+v", got)
	}
}

func TestGroupPendingByPayloadCoalesces(t *testing.T) {
	syncRunAt := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	pending := map[string]templateCategorySync.TemplateCategoryRow{
		"a_utility": {Name: "a_utility", Category: "MARKETING", Status: "APPROVED"},
		"b_utility": {Name: "b_utility", Category: "MARKETING", Status: "APPROVED"},
		"c_ok":      {Name: "c_ok", Category: "UTILITY", Status: "APPROVED"},
	}
	groups := templateCategorySync.GroupPendingByPayload(pending, syncRunAt)
	if len(groups) != 2 {
		t.Fatalf("groups=%d want 2", len(groups))
	}
}
