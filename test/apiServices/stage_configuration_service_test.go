package apiServices_test

import (
	"encoding/json"
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

func TestNormalizeStageConfigurationAcceptsOpenDay(t *testing.T) {
	interval := "1d;30d;30+"
	request := apiModels.StageConfigurationRequest{LenderName: "zapcash", CommType: "whatsapp", Stage: 12, Interval: &interval}
	if err := apiServices.NormalizeStageConfigurationRequest(&request); err != nil {
		t.Fatalf("open day: %v", err)
	}
	if request.Interval == nil || *request.Interval != "1d;30d;30+" {
		t.Fatalf("normalized interval = %v", request.Interval)
	}
	hourOpen := "2h+"
	rejected := apiModels.StageConfigurationRequest{LenderName: "zapcash", CommType: "whatsapp", Stage: 12, Interval: &hourOpen}
	if err := apiServices.NormalizeStageConfigurationRequest(&rejected); err == nil {
		t.Fatal("hour open token was accepted")
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

	times := []string{"15:05", "10:30"}
	timesOnly := apiModels.StageConfigurationRequest{
		LenderName: "zapcash", CommType: "whatsapp", Stage: 3, ExecutionTimes: &times,
	}
	if err := apiServices.NormalizeStageConfigurationRequest(&timesOnly); err != nil {
		t.Fatalf("executionTimes-only request: %v", err)
	}
	if timesOnly.ExecutionTimes == nil || !reflect.DeepEqual(*timesOnly.ExecutionTimes, []string{"10:30", "15:05"}) {
		t.Fatalf("executionTimes-only = %v", timesOnly.ExecutionTimes)
	}
}

func TestStageLockIdentitiesUseCanonicalPreHashOrder(t *testing.T) {
	ordered := apiServices.OrderedStageConfigurationLockIdentities("z|SMS|2", "a|SMS|9", "z|SMS|2")
	if !reflect.DeepEqual(ordered, []string{"a|SMS|9", "z|SMS|2"}) {
		t.Fatalf("order = %v", ordered)
	}
}

func TestFillExecutionTimesKeepsNullJSONShape(t *testing.T) {
	var schedule apiModels.LenderSchedule
	schedule.FillExecutionTimes()
	raw, err := json.Marshal(schedule)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"executionTime1", "executionTime2", "executionTime3"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing %s", key)
		}
		if payload[key] != nil {
			t.Fatalf("%s = %v, want null", key, payload[key])
		}
	}
	times, ok := payload["executionTimes"].([]any)
	if !ok {
		t.Fatalf("executionTimes type = %T", payload["executionTimes"])
	}
	if len(times) != 0 {
		t.Fatalf("executionTimes = %v, want []", times)
	}

	t1 := "15:05:00"
	schedule.ExecutionTime1 = &t1
	schedule.FillExecutionTimes()
	if schedule.ExecutionTime1 == nil || *schedule.ExecutionTime1 != "15:05" {
		t.Fatalf("executionTime1 = %v", schedule.ExecutionTime1)
	}
	if !reflect.DeepEqual(schedule.ExecutionTimes, []string{"15:05"}) {
		t.Fatalf("executionTimes = %v", schedule.ExecutionTimes)
	}
}

func TestNormalizeExecutionTimesSortDedupCompact(t *testing.T) {
	got, err := apiServices.NormalizeExecutionTimes("zapcash", []string{"15:05", "10:30", "10:30", " 08:00 ", ""})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	want := []string{"08:00", "10:30", "15:05"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNormalizeExecutionTimesRejectsNonZapcashAndOverCap(t *testing.T) {
	if _, err := apiServices.NormalizeExecutionTimes("wecredit", []string{"10:00"}); err == nil {
		t.Fatal("expected non-zapcash rejection")
	}
	if _, err := apiServices.NormalizeExecutionTimes("zapcash", []string{"01:00", "02:00", "03:00", "04:00"}); err == nil {
		t.Fatal("expected over-cap rejection")
	}
	if _, err := apiServices.NormalizeExecutionTimes("zapcash", []string{"25:00"}); err == nil {
		t.Fatal("expected invalid hour rejection")
	}
	if _, err := apiServices.NormalizeExecutionTimes("zapcash", []string{"10:30:99"}); err == nil {
		t.Fatal("expected malformed HH:MM:SS rejection")
	}
	if _, err := apiServices.NormalizeExecutionTimes("zapcash", []string{"10:30:00"}); err == nil {
		t.Fatal("expected HH:MM:SS rejection")
	}
}

func TestNormalizeStageConfigurationExecutionTimes(t *testing.T) {
	interval := "1d"
	times := []string{"15:05", "10:30"}
	request := apiModels.StageConfigurationRequest{
		LenderName: "zapcash", CommType: "whatsapp", Stage: 3, Interval: &interval, ExecutionTimes: &times,
	}
	if err := apiServices.NormalizeStageConfigurationRequest(&request); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if request.ExecutionTimes == nil || !reflect.DeepEqual(*request.ExecutionTimes, []string{"10:30", "15:05"}) {
		t.Fatalf("executionTimes = %v", request.ExecutionTimes)
	}
}
