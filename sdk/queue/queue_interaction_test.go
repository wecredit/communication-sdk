package queue

import (
	"testing"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func TestNormalizeErrorPayloadKeepsClient(t *testing.T) {
	payload, err := normalizeErrorPayload(extapimodels.SmsRequestBody{
		Client:  "wecredit",
		Process: "nurture",
		CommId:  "marketing-42",
		Channel: "SMS",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := firstNonEmptyString(payload, "Client"); got != "wecredit" {
		t.Fatalf("Client = %q", got)
	}
	if got := firstNonEmptyString(payload, "CommId"); got != "marketing-42" {
		t.Fatalf("CommId = %q", got)
	}
}

func TestFirstNonEmptyStringFallsBackToProcess(t *testing.T) {
	payload := map[string]interface{}{"Process": "zapcash"}
	if got := firstNonEmptyString(payload, "Client", "client"); got != "" {
		t.Fatalf("Client should be empty, got %q", got)
	}
	client := firstNonEmptyString(payload, "Client", "client")
	if client == "" {
		client = firstNonEmptyString(payload, "Process", "process")
	}
	if client != "zapcash" {
		t.Fatalf("fallback client = %q", client)
	}
}
