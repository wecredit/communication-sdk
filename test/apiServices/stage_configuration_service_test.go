package apiServices_test

import (
	"reflect"
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestNormalizeStageConfiguration(t *testing.T) {
	request := apiModels.StageConfigurationRequest{
		LenderName: " ZapCash ", CommType: "sms", Stage: 2, Interval: " 0D ; monday ; -1d ",
		TemplateStages: []apiModels.StageMappingInput{{SubStage: 10}, {SubStage: 1}},
	}
	if err := apiServices.NormalizeStageConfigurationRequest(&request); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if request.LenderName != "zapcash" || request.CommType != "SMS" || request.Interval != "0d;monday;-1d" {
		t.Fatalf("unexpected normalization: %+v", request)
	}
	want := []apiModels.StageMappingInput{{SubStage: 1}, {SubStage: 10}}
	if !reflect.DeepEqual(request.TemplateStages, want) {
		t.Fatalf("mappings = %+v, want %+v", request.TemplateStages, want)
	}
}

func TestNormalizeStageConfigurationRejectsInvalidInput(t *testing.T) {
	tests := []apiModels.StageConfigurationRequest{
		{LenderName: "x", CommType: "SMS", Stage: -1, Interval: "0d", TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: "", TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: "0d;0d", TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: "0d", TemplateStages: []apiModels.StageMappingInput{{SubStage: 0}}},
		{LenderName: "x", CommType: "SMS", Stage: 1, Interval: "0d", TemplateStages: []apiModels.StageMappingInput{{SubStage: 1}, {SubStage: 1}}},
	}
	for i := range tests {
		if err := apiServices.NormalizeStageConfigurationRequest(&tests[i]); err == nil {
			t.Fatalf("case %d unexpectedly passed", i)
		}
	}
}

func TestStageLockIdentitiesUseCanonicalPreHashOrder(t *testing.T) {
	ordered := apiServices.OrderedStageConfigurationLockIdentities("z|SMS|2", "a|SMS|9", "z|SMS|2")
	if !reflect.DeepEqual(ordered, []string{"a|SMS|9", "z|SMS|2"}) {
		t.Fatalf("order = %v", ordered)
	}
}
