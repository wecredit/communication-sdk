package channelHelper_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func TestPopulateSmsFieldsConvertsTemplateEntityID(t *testing.T) {
	request := extapimodels.SmsRequestBody{}
	channelHelper.PopulateSmsFields(&request, map[string]interface{}{
		"TemplateEntityId": int64(1001234567890123456),
		"TemplateHeader":   "WECRPL",
	})

	if request.TemplateEntityId != "1001234567890123456" {
		t.Fatalf("TemplateEntityId = %q", request.TemplateEntityId)
	}
	if request.TemplateHeader != "WECRPL" {
		t.Fatalf("TemplateHeader = %q", request.TemplateHeader)
	}
}

func TestFetchTemplateDataByReference(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		ProcessName:       "fatakpay",
		Client:            "wecredit",
		Channel:           "SMS",
		Vendor:            "SINCH",
		TemplateReference: "1707177200722881790",
	}
	templates := map[string]map[string]interface{}{
		"Process:fatakpay|Stage:1.01|Client:wecredit|Channel:SMS|Vendor:SINCH": {
			"IsActive":      variables.Active,
			"DltTemplateId": int64(1707177200722881790),
			"TemplateName":  "FATAKPAY_1",
			"Client":        "wecredit",
			"Channel":       "SMS",
			"Vendor":        "SINCH",
			"Process":       "fatakpay",
			"TemplateText":  "hello {#var#}",
		},
	}

	data, vendor, err := channelHelper.FetchTemplateDataByReference(msg, templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vendor != "SINCH" {
		t.Fatalf("vendor = %q, want SINCH", vendor)
	}
	if data["TemplateText"] != "hello {#var#}" {
		t.Fatalf("TemplateText = %v", data["TemplateText"])
	}
}

func TestFetchTemplateDataByReferenceRequiresProcess(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		Client: "wecredit", Channel: "SMS", Vendor: "PINNACLE", TemplateReference: "1777178764367201169",
	}

	_, _, err := channelHelper.FetchTemplateDataByReference(msg, map[string]map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "process name is required") {
		t.Fatalf("error = %v, want missing-process validation", err)
	}
}

func TestFetchTemplateDataByReferenceSeparatesProcesses(t *testing.T) {
	templates := map[string]map[string]interface{}{
		"branch": {
			"IsActive": variables.Active, "DltTemplateId": int64(1777178764367201169),
			"Client": "wecredit", "Channel": "SMS", "Vendor": "PINNACLE", "Process": "Branch", "TemplateText": "production",
		},
		"branch-test": {
			"IsActive": variables.Active, "DltTemplateId": int64(1777178764367201169),
			"Client": "wecredit", "Channel": "SMS", "Vendor": "PINNACLE", "Process": "Branch_test", "TemplateText": "test",
		},
	}
	msg := sdkModels.CommApiRequestBody{
		ProcessName: "Branch_test", Client: "wecredit", Channel: "SMS", Vendor: "PINNACLE",
		TemplateReference: "1777178764367201169",
	}

	data, _, err := channelHelper.FetchTemplateDataByReference(msg, templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["TemplateText"] != "test" {
		t.Fatalf("TemplateText = %v, want test process template", data["TemplateText"])
	}
}

func TestResolveTemplateDataUsesReferenceWhenPresent(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		ProcessName:       "fatakpay",
		Stage:             9.99,
		Client:            "wecredit",
		Channel:           "SMS",
		Vendor:            "SINCH",
		TemplateReference: "1707177200722881790",
	}
	templates := map[string]map[string]interface{}{
		"Process:fatakpay|Stage:1.01|Client:wecredit|Channel:SMS|Vendor:SINCH": {
			"IsActive":      variables.Active,
			"DltTemplateId": int64(1707177200722881790),
			"TemplateName":  "FATAKPAY_1",
			"Client":        "wecredit",
			"Channel":       "SMS",
			"Vendor":        "SINCH",
			"Process":       "fatakpay",
			"TemplateText":  "referenced",
		},
	}

	data, _, err := channelHelper.ResolveTemplateData(msg, templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["TemplateText"] != "referenced" {
		t.Fatalf("TemplateText = %v", data["TemplateText"])
	}
}

func TestFetchTemplateDataByReferenceUsesTemplateNameForRCS(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		ProcessName:       "zapcash",
		Client:            "zapcash",
		Channel:           "RCS",
		Vendor:            "PINNACLE",
		TemplateReference: "TPL_RCS_1",
	}
	templates := map[string]map[string]interface{}{
		"Process:zapcash|Stage:9.01|Client:zapcash|Channel:RCS|Vendor:PINNACLE": {
			"IsActive":      variables.Active,
			"DltTemplateId": int64(1707177200722881790),
			"TemplateName":  "TPL_RCS_1",
			"Client":        "zapcash",
			"Channel":       "RCS",
			"Vendor":        "PINNACLE",
			"Process":       "zapcash",
			"TemplateText":  "rcs template",
		},
	}

	data, vendor, err := channelHelper.FetchTemplateDataByReference(msg, templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vendor != "PINNACLE" {
		t.Fatalf("vendor = %q, want PINNACLE", vendor)
	}
	if data["TemplateName"] != "TPL_RCS_1" {
		t.Fatalf("TemplateName = %v", data["TemplateName"])
	}
}

func TestFetchTemplateDataDoesNotCrossVendorWhenVendorExplicit(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		ProcessName: "nurture",
		Stage:       1.01,
		Client:      "wecredit",
		Channel:     "SMS",
		Vendor:      "PINNACLE",
	}
	templates := map[string]map[string]interface{}{
		"Process:nurture|Stage:1.01|Client:wecredit|Channel:SMS|Vendor:SINCH": {
			"IsActive":     variables.Active,
			"TemplateText": "hello",
		},
	}

	_, vendor, err := channelHelper.FetchTemplateData(msg, templates)
	if err == nil {
		t.Fatal("expected error when explicit vendor template is missing")
	}
	if vendor != "PINNACLE" {
		t.Fatalf("vendor = %q, want PINNACLE", vendor)
	}
	if !strings.Contains(err.Error(), "PINNACLE") {
		t.Fatalf("error should mention explicit vendor, got %v", err)
	}
}

func TestFetchTemplateDataSameVendorStageFallback(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		ProcessName: "nurture",
		Stage:       1.01,
		Client:      "wecredit",
		Channel:     "SMS",
		Vendor:      "PINNACLE",
	}
	templates := map[string]map[string]interface{}{
		"Process:nurture|Stage:1.02|Client:wecredit|Channel:SMS|Vendor:PINNACLE": {
			"IsActive":     variables.Active,
			"TemplateText": "fallback",
		},
	}

	data, vendor, err := channelHelper.FetchTemplateData(msg, templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vendor != "PINNACLE" {
		t.Fatalf("vendor = %q, want PINNACLE", vendor)
	}
	if data["TemplateText"] != "fallback" {
		t.Fatalf("TemplateText = %v", data["TemplateText"])
	}
}
