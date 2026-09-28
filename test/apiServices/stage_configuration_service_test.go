package apiServices_test

import (
	"reflect"
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestNormalizeStageConfiguration(t *testing.T) {
	interval := " 0D ; monday ; -1d "
	request := apiModels.StageConfigurationRequest{
		LenderName: " ZapCash ", CommType: "sms", Stage: 2, Interval: &interval,
		TemplateStages: []apiModels.StageMappingInput{{SubStage: 10}, {SubStage: 1}},
	}
	if err := apiServices.NormalizeStageConfigurationRequest(&request); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if request.LenderName != "zapcash" || request.CommType != "SMS" || request.Interval == nil || *request.Interval != "0d;monday;-1d" {
		t.Fatalf("unexpected normalization: %+v", request)
	}
	want := []apiModels.StageMappingInput{{SubStage: 1}, {SubStage: 10}}
	if !reflect.DeepEqual(request.TemplateStages, want) {
		t.Fatalf("mappings = %+v, want %+v", request.TemplateStages, want)
	}
}

func TestNormalizeStageConfigurationRejectsInvalidInput(t *testing.T) {
	validInterval := "0d"
	emptyInterval := ""
	duplicateInterval := "0d;0d"
	tests := []apiModels.StageConfigurationRequest{
		{LenderName: "x", CommType: "SMS", Stage: -1, Interval: &validInterval, TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: &emptyInterval, TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: &duplicateInterval, TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: &validInterval, TemplateStages: []apiModels.StageMappingInput{{SubStage: 0}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: &validInterval, TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}, {SubStage: 1}}},
		{LenderName: "x", CommType: "SMS", Stage: 1},
	}
	for i := range tests {
		if err := apiServices.NormalizeStageConfigurationRequest(&tests[i]); err == nil {
			t.Fatalf("case %d unexpectedly passed", i)
		}
	}
}

func TestNormalizeStageConfigurationAllowsIndependentResources(t *testing.T) {
	interval := " 1D "
	scheduleOnly := apiModels.StageConfigurationRequest{LenderName: "zapcash", CommType: "sms", Stage: 1, Interval: &interval}
	if err := apiServices.NormalizeStageConfigurationRequest(&scheduleOnly); err != nil {
		t.Fatalf("schedule-only request: %v", err)
	}
	if scheduleOnly.Interval == nil || *scheduleOnly.Interval != "1d" {
		t.Fatalf("normalized schedule interval = %v", scheduleOnly.Interval)
	}

	mappingOnly := apiModels.StageConfigurationRequest{
		LenderName: "zapcash", CommType: "whatsapp", Stage: 10,
		TemplateStages: []apiModels.StageMappingInput{{SubStage: 2}, {SubStage: 1}},
	}
	if err := apiServices.NormalizeStageConfigurationRequest(&mappingOnly); err != nil {
		t.Fatalf("mapping-only request: %v", err)
	}
	if mappingOnly.Interval != nil || mappingOnly.TemplateStages[0].SubStage != 1 {
		t.Fatalf("unexpected mapping-only normalization: %+v", mappingOnly)
	}
}

func TestStageLockIdentitiesUseCanonicalPreHashOrder(t *testing.T) {
	ordered := apiServices.OrderedStageConfigurationLockIdentities("z|SMS|2", "a|SMS|9", "z|SMS|2")
	if !reflect.DeepEqual(ordered, []string{"a|SMS|9", "z|SMS|2"}) {
		t.Fatalf("order = %v", ordered)
	}
}
