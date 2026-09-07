package push_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/push"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestEligibilityIdentityUsesUserIdWhenPresent(t *testing.T) {
	got := push.EligibilityIdentity(sdkModels.CommApiRequestBody{
		UserId:  "user-42",
		EventId: "event-hash",
	})
	if got != "user-42" {
		t.Fatalf("EligibilityIdentity = %q, want user-42", got)
	}
}

func TestEligibilityIdentityFallsBackToEventIdPermanently(t *testing.T) {
	got := push.EligibilityIdentity(sdkModels.CommApiRequestBody{
		UserId:  "  ",
		EventId: "event-hash",
	})
	if got != "event-hash" {
		t.Fatalf("EligibilityIdentity = %q, want EventId (permanent app_number path)", got)
	}

	identity := push.PushIdentityFromRequest(sdkModels.CommApiRequestBody{
		Client:       "zapcash",
		EventId:      "event-hash",
		ProcessName:  "OFFER",
		CampaignDate: "2026-09-07",
	}, "tmpl")
	if identity.EligibilityIdentity != "event-hash" {
		t.Fatalf("EligibilityIdentity = %q", identity.EligibilityIdentity)
	}
	if identity.CampaignDate != "2026-09-07" {
		t.Fatalf("CampaignDate = %q", identity.CampaignDate)
	}
}
