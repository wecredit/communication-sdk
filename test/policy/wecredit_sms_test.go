package policy_test

import (
	"testing"
	"time"

	smspolicy "github.com/wecredit/communication-sdk/sdk/policy"
)

func istTime(t *testing.T, value string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestWeCreditSMSCampaignDateAndCutoff(t *testing.T) {
	tests := []struct {
		name         string
		campaignDate string
		now          string
		want         string
	}{
		{"before cutoff", "2026-08-23", "2026-08-23 19:59:59", smspolicy.DecisionAllowed},
		{"at cutoff", "2026-08-23", "2026-08-23 20:00:00", smspolicy.DecisionCutoff},
		{"after cutoff", "2026-08-23", "2026-08-23 20:00:01", smspolicy.DecisionCutoff},
		{"expired", "2026-08-23", "2026-08-24 09:00:00", smspolicy.DecisionExpired},
		{"future", "2026-08-25", "2026-08-24 09:00:00", smspolicy.DecisionCampaignDateInvalid},
		{"missing", "", "2026-08-24 09:00:00", smspolicy.DecisionCampaignDateInvalid},
		{"malformed", "24-08-2026", "2026-08-24 09:00:00", smspolicy.DecisionCampaignDateInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := smspolicy.Evaluate("marketing", 42, "SMS", test.campaignDate, istTime(t, test.now))
			if got.Code != test.want {
				t.Fatalf("decision = %s, want %s", got.Code, test.want)
			}
		})
	}
}

func TestNonMarketingSMSIsUnaffected(t *testing.T) {
	got := smspolicy.Evaluate("", 0, "SMS", "", istTime(t, "2026-08-23 21:00:00"))
	if !got.Allowed() {
		t.Fatalf("non-marketing decision = %s, want allowed", got.Code)
	}
}

func TestRateLimitCrossingCutoffRequiresSecondDecision(t *testing.T) {
	early := smspolicy.Evaluate("marketing", 42, "SMS", "2026-08-23", istTime(t, "2026-08-23 19:59:59"))
	if !early.Allowed() {
		t.Fatalf("early decision = %s, want allowed", early.Code)
	}
	final := smspolicy.Evaluate("marketing", 42, "SMS", "2026-08-23", istTime(t, "2026-08-23 20:00:01"))
	if final.Code != smspolicy.DecisionCutoff {
		t.Fatalf("final decision = %s, want cutoff", final.Code)
	}
}
