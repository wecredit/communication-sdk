package templateCategorySync_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/services/templateCategorySync"
)

func TestComputeCategoryUpdate(t *testing.T) {
	ok := templateCategorySync.ComputeCategoryUpdate("foo_utility_v1", "UTILITY", "APPROVED")
	if !ok.IsActive || ok.Error != "" || ok.TouchCategoryOn {
		t.Fatalf("utility+UTILITY+APPROVED = %+v", ok)
	}

	mismatch := templateCategorySync.ComputeCategoryUpdate("foo_utility_v1", "MARKETING", "APPROVED")
	if mismatch.IsActive || mismatch.Error == "" || !mismatch.TouchCategoryOn {
		t.Fatalf("utility+MARKETING mismatch = %+v", mismatch)
	}
	if mismatch.ProviderCategory != "MARKETING" {
		t.Fatalf("ProviderCategory = %q, want MARKETING", mismatch.ProviderCategory)
	}

	inactive := templateCategorySync.ComputeCategoryUpdate("promo_v1", "MARKETING", "PENDING")
	if inactive.IsActive {
		t.Fatalf("PENDING should deactivate: %+v", inactive)
	}
}

func TestParseVendorPanelsJSON(t *testing.T) {
	t.Parallel()
	raw := `[
		{"api_source":"times","base_url":"https://wecredit1.timespanel.in","api_id":"tok1"},
		{"api_source":"pinnacle","base_url":"https://partnersv1.pinbot.ai/v3","api_id":"1000"}
	]`
	panels, err := templateCategorySync.ParseVendorPanelsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(panels) != 2 {
		t.Fatalf("len=%d", len(panels))
	}
	if panels[0].ApiSource != "times" || panels[0].BaseURL == "" || panels[0].ApiID != "tok1" {
		t.Fatalf("times panel = %+v", panels[0])
	}
	if panels[1].ApiSource != "pinnacle" {
		t.Fatalf("pinnacle panel = %+v", panels[1])
	}
}

func TestParseVendorPanelsJSONInvalid(t *testing.T) {
	t.Parallel()
	_, err := templateCategorySync.ParseVendorPanelsJSON(`{not-json`)
	if err == nil {
		t.Fatal("expected error")
	}
}
