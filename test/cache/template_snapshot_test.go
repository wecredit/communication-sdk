package cache_test

import (
	"math"
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
)

func TestCanonicalTemplateStage(t *testing.T) {
	tests := []struct {
		stage float64
		want  string
	}{
		{stage: 0, want: "0.00"},
		{stage: 1, want: "1.00"},
		{stage: 2.25, want: "2.25"},
	}
	for _, test := range tests {
		got, err := cache.CanonicalTemplateStage(test.stage)
		if err != nil {
			t.Fatalf("CanonicalTemplateStage(%v) error = %v", test.stage, err)
		}
		if got != test.want {
			t.Fatalf("CanonicalTemplateStage(%v) = %q, want %q", test.stage, got, test.want)
		}
	}
	for _, invalid := range []float64{1.234, math.NaN(), math.Inf(1)} {
		if _, err := cache.CanonicalTemplateStage(invalid); err == nil {
			t.Fatalf("CanonicalTemplateStage(%v) accepted invalid Stage", invalid)
		}
	}
}

func TestBuildAndInstallTemplateSnapshot(t *testing.T) {
	rows := []apiModels.Templatedetails{
		stageTemplate(1, 2.25, "SMS"),
		{
			Id: 2, Process: "MARKETING", Client: "wecredit", Channel: "WHATSAPP",
			Vendor: "TIMES", TemplateName: "payment_due", IsActive: true,
		},
	}
	snapshot, err := cache.BuildTemplateSnapshot(rows)
	if err != nil {
		t.Fatalf("BuildTemplateSnapshot() error = %v", err)
	}
	if len(snapshot.Templates) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if err := cache.InstallTemplateSnapshot(snapshot); err != nil {
		t.Fatalf("InstallTemplateSnapshot() error = %v", err)
	}
	installed, found := cache.CurrentTemplateSnapshot()
	if !found || installed != snapshot {
		t.Fatal("installed snapshot was not atomically exposed")
	}
}

func TestSnapshotRejectsDuplicateActiveResolutionKey(t *testing.T) {
	rows := []apiModels.Templatedetails{
		stageTemplate(10, 1.00, "SMS"),
		stageTemplate(11, 1.00, "SMS"),
	}
	if _, err := cache.BuildTemplateSnapshot(rows); err == nil {
		t.Fatal("BuildTemplateSnapshot() accepted duplicate active templates")
	}
}

func TestSnapshotIgnoresInactiveDuplicateResolutionKey(t *testing.T) {
	active := stageTemplate(14, 2.00, "RCS")
	active.Process = "OLYV"
	active.TemplateName = "olyv_stage_2_5_e_mar"

	inactive := active
	inactive.Id = 15
	inactive.TemplateName = "olyv_stage_2_h_5_mar"
	inactive.IsActive = false

	snapshot, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{active, inactive})
	if err != nil {
		t.Fatalf("inactive duplicate blocked snapshot: %v", err)
	}
	if len(snapshot.Templates) != 1 {
		t.Fatalf("snapshot contains %d templates, want only the active row", len(snapshot.Templates))
	}
}

func TestSnapshotAllowsSameReferenceForDifferentProcesses(t *testing.T) {
	branch := referenceSMSTemplate(16, "Branch")
	branchTest := referenceSMSTemplate(17, "Branch_test")

	snapshot, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{branch, branchTest})
	if err != nil {
		t.Fatalf("different processes should have independent reference identities: %v", err)
	}
	if len(snapshot.Templates) != 2 {
		t.Fatalf("snapshot contains %d templates, want 2", len(snapshot.Templates))
	}
}

func TestSnapshotRejectsSameReferenceWithinProcess(t *testing.T) {
	first := referenceSMSTemplate(18, "Branch")
	second := referenceSMSTemplate(19, " branch ")

	if _, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{first, second}); err == nil {
		t.Fatal("same-process reference duplicate was accepted")
	}
}

func TestFailedBuildRetainsLastKnownGoodSnapshot(t *testing.T) {
	good, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{stageTemplate(20, 3.00, "SMS")})
	if err != nil {
		t.Fatalf("build initial snapshot: %v", err)
	}
	if err := cache.InstallTemplateSnapshot(good); err != nil {
		t.Fatalf("install initial snapshot: %v", err)
	}

	bad, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{{
		Id: 21, Process: "P", Client: "wecredit", Channel: "VOICE", Vendor: "VENDOR", IsActive: true,
	}})
	if err != nil {
		t.Fatalf("unresolvable rows should be reported without invalidating the snapshot: %v", err)
	}
	if len(bad.Findings) != 1 {
		t.Fatalf("unresolvable finding missing: %+v", bad)
	}

	duplicateRows := []apiModels.Templatedetails{
		stageTemplate(22, 4.00, "SMS"),
		stageTemplate(23, 4.00, "SMS"),
	}
	if _, err := cache.BuildTemplateSnapshot(duplicateRows); err == nil {
		t.Fatal("expected invalid snapshot build to fail")
	}
	installed, found := cache.CurrentTemplateSnapshot()
	if !found || installed != good {
		t.Fatalf("last-known-good snapshot was replaced: %+v", installed)
	}
}

func stageTemplate(id int, stage float64, channel string) apiModels.Templatedetails {
	return apiModels.Templatedetails{
		Id: id, Process: "COLLECTION", Stage: &stage, Client: "wecredit",
		Channel: channel, Vendor: "SINCH", IsActive: true,
	}
}

func referenceSMSTemplate(id int, process string) apiModels.Templatedetails {
	return apiModels.Templatedetails{
		Id: id, Process: process, Client: "wecredit", Channel: "SMS", Vendor: "PINNACLE",
		DltTemplateId: 1777178764367201169, IsActive: true,
	}
}
