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

func TestMapMarketingWhatsappMysqlOutputLenderShaped(t *testing.T) {
	data := sdkModels.CommApiRequestBody{
		CommId:            "WC-TEST",
		Mobile:            "9876543210",
		Vendor:            "SINCH",
		TemplateReference: "tpl_wa",
		AppId:             "row-app",
	}
	out := services.MapMarketingWhatsappMysqlOutput(data, map[string]interface{}{
		"TransactionId":   "txn-1",
		"IsSent":          true,
		"ResponseMessage": "ok",
		"AppId":           "template-app",
		"MobileNumber":    "9999999999",
	})

	if out["CommId"] != "WC-TEST" {
		t.Fatalf("CommId = %v", out["CommId"])
	}
	if out["MobileNumber"] != "9999999999" {
		t.Fatalf("MobileNumber = %v, want provider/mobile override", out["MobileNumber"])
	}
	if out["IsSent"] != true {
		t.Fatalf("IsSent = %v", out["IsSent"])
	}
	if _, ok := out["AppId"]; ok {
		t.Fatal("MySQL WhatsappOutput must not include AppId")
	}
	if _, ok := out["SourceRowId"]; ok {
		t.Fatal("MySQL WhatsappOutput must not include Marketing SourceRowId")
	}
	if _, ok := out["EventId"]; ok {
		t.Fatal("MySQL WhatsappOutput must not include Marketing EventId")
	}
}

func TestMapWhatsappMysqlOutputAllowlist(t *testing.T) {
	out := services.MapWhatsappMysqlOutput(map[string]interface{}{
		"CommId":          "WC-TEST",
		"Vendor":          "TIMES",
		"MobileNumber":    "9876543210",
		"IsSent":          true,
		"TransactionId":   "txn-1",
		"ResponseMessage": "ok",
		"PaymentLink":     "",
		"TemplateName":    "tpl_wa",
		"AppId":           "must-not-be-written",
		"RawPayload":      `{"request":true}`,
		"RawResponse":     `{"response":true}`,
	})

	if len(out) != 8 {
		t.Fatalf("allowlisted column count = %d, want 8", len(out))
	}
	for _, column := range []string{"AppId", "RawPayload", "RawResponse"} {
		if _, ok := out[column]; ok {
			t.Fatalf("legacy MySQL output must not include %s", column)
		}
	}
}

func TestMapMarketingWhatsappMysqlOutputExcludesMarketingOnlyRawFields(t *testing.T) {
	data := sdkModels.CommApiRequestBody{
		CommId: "WC-TEST",
		Mobile: "9876543210",
		Vendor: "SINCH",
	}
	out := services.MapMarketingWhatsappMysqlOutput(data, map[string]interface{}{
		"RawPayload":  `{"request":true}`,
		"RawResponse": `{"response":true}`,
	})

	if _, ok := out["RawPayload"]; ok {
		t.Fatal("MySQL WhatsappOutput must not include RawPayload")
	}
	if _, ok := out["RawResponse"]; ok {
		t.Fatal("MySQL WhatsappOutput must not include RawResponse")
	}
}
