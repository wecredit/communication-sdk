package monitoring_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/services/monitoring"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestBuildMonitoringPayloadPinsVendorAndReplacesPII(t *testing.T) {
	production := sdkModels.CommApiRequestBody{CommId: "PRODUCTION-ID", Mobile: "9000000000", Client: "zapcash", ProcessName: "ZAPCASH", Channel: "SMS", Stage: 12.01, Vendor: "SINCH", CustomerName: "Production User"}
	result := monitoring.AcceptedResult{Payload: production, ResolvedVendor: "PINNACLE", ResolvedTemplate: "12345", TemplateVariables: "CustomerName,LoanId"}
	payload, err := monitoring.BuildMonitoringPayload(monitoring.Profile{CustomerName: "Monitor User", LoanID: "MON-LOAN"}, result, "9899074649", "dedup-key")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Mobile != "9899074649" || payload.CustomerName != "Monitor User" || payload.LoanId != "MON-LOAN" {
		t.Fatalf("monitoring profile was not applied: %+v", payload)
	}
	if payload.Vendor != "PINNACLE" || payload.TemplateReference != "12345" {
		t.Fatalf("resolved production route was not pinned: %+v", payload)
	}
	if !payload.IsMonitorCopy || payload.Client != "zapcash" || payload.ProcessName != "ZAPCASH" {
		t.Fatalf("monitoring identity missing: %+v", payload)
	}
	if payload.CommId == production.CommId || !strings.HasPrefix(payload.CommId, "ZAPCASH-MON-") {
		t.Fatalf("unexpected monitoring CommId %q", payload.CommId)
	}
}

func TestBuildMonitoringPayloadRejectsMissingProfileVariable(t *testing.T) {
	_, err := monitoring.BuildMonitoringPayload(monitoring.Profile{}, monitoring.AcceptedResult{Payload: sdkModels.CommApiRequestBody{Channel: "RCS", Stage: 1}, ResolvedVendor: "PINNACLE", ResolvedTemplate: "template", TemplateVariables: "CustomerName"}, "9899074649", "dedup-key")
	if err == nil {
		t.Fatal("missing required profile field must fail before audit/publish")
	}
}
