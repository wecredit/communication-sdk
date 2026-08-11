package sinchSmsPayload

import (
	"testing"

	models "github.com/wecredit/communication-sdk/sdk/models"
)

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
			username, _, _, _, err := sinchSMSCredentials(test.client, config)
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
	if _, _, _, _, err := sinchSMSCredentials("unknown", models.Config{}); err == nil {
		t.Fatal("expected unknown client to be rejected")
	}
	if _, _, _, _, err := sinchSMSCredentials("creditsea", models.Config{}); err == nil {
		t.Fatal("expected incomplete credentials to be rejected")
	}
}
