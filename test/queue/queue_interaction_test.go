package queue_test

import (
	"testing"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
)

func TestNormalizeErrorPayloadKeepsClient(t *testing.T) {
	payload, err := queue.NormalizeErrorPayload(extapimodels.SmsRequestBody{
		Client:  "wecredit",
		Process: "nurture",
		CommId:  "marketing-42",
		Channel: "SMS",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := queue.FirstNonEmptyString(payload, "Client"); got != "wecredit" {
		t.Fatalf("Client = %q", got)
	}
	if got := queue.FirstNonEmptyString(payload, "CommId"); got != "marketing-42" {
		t.Fatalf("CommId = %q", got)
	}
}

func TestFirstNonEmptyStringFallsBackToProcess(t *testing.T) {
	payload := map[string]interface{}{"Process": "zapcash"}
	if got := queue.FirstNonEmptyString(payload, "Client", "client"); got != "" {
		t.Fatalf("Client should be empty, got %q", got)
	}
	client := queue.FirstNonEmptyString(payload, "Client", "client")
	if client == "" {
		client = queue.FirstNonEmptyString(payload, "Process", "process")
	}
	if client != "zapcash" {
		t.Fatalf("fallback client = %q", client)
	}
}
