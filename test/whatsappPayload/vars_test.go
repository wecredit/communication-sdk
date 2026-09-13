package whatsappPayload_test

import (
	"testing"

	whatsappPayload "github.com/wecredit/communication-sdk/internal/channels/whatsapp/whatsappPayload"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func TestPositionalBodyParams(t *testing.T) {
	got := whatsappPayload.PositionalBodyParams(" a ,b, c ")
	if len(got) != 3 || got[0]["text"] != "a" || got[2]["text"] != "c" {
		t.Fatalf("got %#v", got)
	}
	if whatsappPayload.PositionalBodyParams("") != nil {
		t.Fatal("empty should be nil")
	}
}

func TestButtonMobileSubstitute(t *testing.T) {
	req := extapimodels.WhatsappRequestBody{Mobile: "9876543210", DynamicMobile: "TOKEN123"}
	if got := whatsappPayload.ButtonMobileSubstitute(req); got != "TOKEN123" {
		t.Fatalf("got %q", got)
	}
	req.DynamicMobile = ""
	if got := whatsappPayload.ButtonMobileSubstitute(req); got != "9876543210" {
		t.Fatalf("fallback got %q", got)
	}
}
