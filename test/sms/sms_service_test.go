package sms_test

import (
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/channels/sms"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	smspolicy "github.com/wecredit/communication-sdk/sdk/policy"
)

func TestTerminalReplayResultPreservesDLTTemplateReference(t *testing.T) {
	result := sms.TerminalReplayResult(sdkModels.CommApiRequestBody{
		TemplateReference: "1707177364212095135",
	}, "", "")

	if result.DBData["DltTemplateId"] != int64(1707177364212095135) {
		t.Fatalf("DltTemplateId = %#v, want expected DLT ID", result.DBData["DltTemplateId"])
	}
}

func TestTerminalReplayResultLeavesMalformedDLTTemplateReferenceUnset(t *testing.T) {
	result := sms.TerminalReplayResult(sdkModels.CommApiRequestBody{
		TemplateReference: "not-a-dlt-id",
	}, "", "")

	if result.DBData["DltTemplateId"] != int64(0) {
		t.Fatalf("DltTemplateId = %#v, want int64(0)", result.DBData["DltTemplateId"])
	}
}

func TestComplianceBlockedResultPreservesDLTTemplateReference(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	restoreClock := smspolicy.SetClockForTest(func() time.Time {
		return time.Date(2026, 8, 24, 20, 0, 0, 0, loc)
	})
	t.Cleanup(restoreClock)

	result, err := sms.SendSmsByProcess(sdkModels.CommApiRequestBody{
		Source:            "marketing",
		SourceRowId:       42,
		Channel:           "SMS",
		CampaignDate:      "2026-08-24",
		TemplateReference: "1707177364212095135",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, ok := result.DBData["DltTemplateId"].(int64)
	if !ok || got != 1707177364212095135 {
		t.Fatalf("blocked DltTemplateId = %#v, want int64(1707177364212095135)", result.DBData["DltTemplateId"])
	}
}

func TestTerminalReplayResultPreservesTerminalAuditIdentity(t *testing.T) {
	result := sms.TerminalReplayResult(sdkModels.CommApiRequestBody{
		CommId:            "comm-42",
		Vendor:            "SINCH",
		TemplateReference: "1707177364212095135",
	}, "provider-transaction-42", "")

	if result.DBData["CommId"] != "comm-42" {
		t.Fatalf("CommId = %#v, want comm-42", result.DBData["CommId"])
	}
	if result.DBData["TransactionId"] != "provider-transaction-42" {
		t.Fatalf("TransactionId = %#v, want provider-transaction-42", result.DBData["TransactionId"])
	}
	if result.DBData["IsSent"] != 1 {
		t.Fatalf("IsSent = %#v, want 1", result.DBData["IsSent"])
	}
	if result.DBData["DltTemplateId"] != int64(1707177364212095135) {
		t.Fatalf("DltTemplateId = %#v, want expected DLT ID", result.DBData["DltTemplateId"])
	}
}
