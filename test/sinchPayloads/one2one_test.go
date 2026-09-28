package sinchPayloads_test

import (
	"testing"

	sinchSmsPayload "github.com/wecredit/communication-sdk/internal/channels/sms/sinch/sinchPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	models "github.com/wecredit/communication-sdk/sdk/models"
)

func TestGetTemplatePayloadUsesTemplateHeader(t *testing.T) {
	config := models.Config{
		SinchSmsApiUserName: "wecredit-user", SinchSmsApiPassword: "wecredit-password",
		SinchSmsApiAppID: "wecredit-app", SinchSmsApiSender: "WECRPL",
	}
	payload, err := sinchSmsPayload.GetTemplatePayload(extapimodels.SmsRequestBody{
		Client: "wecredit", Process: "fatakpay", Description: "test",
		TemplateHeader: "WECRQT", DltTemplateId: 1707177200722881790,
		TemplateCategory: "3", TemplateText: "hello", Mobile: "9876543210",
	}, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload["from"] != "WECRQT" {
		t.Fatalf("from = %v, want WECRQT", payload["from"])
	}
}

func TestGetTemplatePayloadDoesNotReapplyPlaceholders(t *testing.T) {
	config := models.Config{
		SinchSmsApiUserName: "wecredit-user", SinchSmsApiPassword: "wecredit-password",
		SinchSmsApiAppID: "wecredit-app", SinchSmsApiSender: "WECRPL",
	}
	// Substitution happens in sms.SendSmsByProcess; payload builder must pass text through.
	payload, err := sinchSmsPayload.GetTemplatePayload(extapimodels.SmsRequestBody{
		Client: "wecredit", Process: "fatakpay", Description: "test",
		TemplateHeader: "WECRQT", DltTemplateId: 1707177200722881790,
		TemplateCategory: "3", TemplateText: "already resolved {#var#} left alone",
		TemplateVariables: "urg", Mobile: "9876543210",
	}, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload["text"] != "already resolved {#var#} left alone" {
		t.Fatalf("text = %v", payload["text"])
	}
}

func TestApplySinchTemplateVariablesWrapper(t *testing.T) {
	text, err := sinchSmsPayload.ApplySinchTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:      "Apply here {#var#} WeCredit",
		TemplateVariables: "PaymentLink",
		PaymentLink:       "https://example.com/apply",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Apply here https://example.com/apply WeCredit" {
		t.Fatalf("text = %q", text)
	}
}

func TestSinchSMSCredentialsByClient(t *testing.T) {
	config := models.Config{
		SinchSmsApiUserName: "wecredit-user", SinchSmsApiPassword: "wecredit-password", SinchSmsApiAppID: "wecredit-app", SinchSmsApiSender: "WECRED",
		CreditSeaSinchSmsApiUserName: "creditsea-user", CreditSeaSinchSmsApiPassword: "creditsea-password", CreditSeaSinchSmsApiAppID: "creditsea-app", CreditSeaSinchSmsApiSender: "CRDSEA",
	}

	for _, test := range []struct{ client, wantUser string }{
		{"wecredit", "wecredit-user"},
		{"creditsea", "creditsea-user"},
	} {
		t.Run(test.client, func(t *testing.T) {
			username, _, _, _, err := sinchSmsPayload.SinchSMSCredentials(test.client, config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if username != test.wantUser {
				t.Fatalf("got username %q, want %q", username, test.wantUser)
			}
		})
	}
}

func TestSinchSMSCredentialsRejectUnknownOrIncompleteClient(t *testing.T) {
	if _, _, _, _, err := sinchSmsPayload.SinchSMSCredentials("unknown", models.Config{}); err == nil {
		t.Fatal("expected unknown client to be rejected")
	}
	if _, _, _, _, err := sinchSmsPayload.SinchSMSCredentials("creditsea", models.Config{}); err == nil {
		t.Fatal("expected incomplete credentials to be rejected")
	}
}
