package consumerServices_test

import (
	"testing"
	"time"

	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestMapMarketingWhatsappOutputHermesParityCols(t *testing.T) {
	scheduled := time.Date(2026, 9, 11, 10, 15, 0, 0, time.UTC)
	data := sdkModels.CommApiRequestBody{
		SourceRowId:            99,
		EventId:                "marketing-99",
		CommId:                 "WC-TEST",
		Mobile:                 "9876543210",
		Vendor:                 "SINCH",
		ProcessName:            "camp_a",
		Client:                 "wecredit",
		TemplateReference:      "tpl_wa",
		AppId:                  "row-app",
		Tag1:                   "t1",
		Tag2:                   "t2",
		DynamicMobile:          "9876543210",
		TemplateVariableValues: "one,two,three",
		ScheduledAt:            scheduled,
	}
	out := services.MapMarketingWhatsappOutput(data, map[string]interface{}{
		"TransactionId":   "txn-1",
		"IsSent":          1,
		"ResponseMessage": "ok",
		"AppId":           "template-app",
		"RawPayload":      `{"x":1}`,
	})

	if out["AppId"] != "template-app" {
		t.Fatalf("AppId = %v, want template-app (resolved send AppId)", out["AppId"])
	}
	if out["TemplateName"] != "tpl_wa" {
		t.Fatalf("TemplateName = %v", out["TemplateName"])
	}
	if out["VariablesValue"] != "one,two,three" {
		t.Fatalf("VariablesValue = %v", out["VariablesValue"])
	}
	if _, ok := out["WA_send"]; ok {
		t.Fatal("WA_send must not be written to output")
	}
	if out["DynamicMobile"] != "9876543210" {
		t.Fatalf("DynamicMobile = %v", out["DynamicMobile"])
	}
	if _, ok := out["ScheduledAt"]; !ok {
		t.Fatal("ScheduledAt missing")
	}
	if out["TransactionId"] != "txn-1" || out["MessageId"] != "txn-1" {
		t.Fatalf("txn = %v / %v", out["TransactionId"], out["MessageId"])
	}
}
