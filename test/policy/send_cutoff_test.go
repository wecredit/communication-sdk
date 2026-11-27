package policy_test

import (
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/sdk/policy"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func TestParseZapCashSendCutoffHourIST(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", policy.DefaultZapCashSendCutoffHourIST},
		{"  ", policy.DefaultZapCashSendCutoffHourIST},
		{"20", 20},
		{"0", 0},
		{"23", 23},
		{"-1", policy.ZapCashSendCutoffDisabledHour},
		{"24", policy.DefaultZapCashSendCutoffHourIST},
		{"abc", policy.DefaultZapCashSendCutoffHourIST},
	}
	for _, test := range tests {
		if got := policy.ParseZapCashSendCutoffHourIST(test.raw); got != test.want {
			t.Fatalf("ParseZapCashSendCutoffHourIST(%q)=%d want %d", test.raw, got, test.want)
		}
	}
}

func TestEvaluateZapCashSendCutoff(t *testing.T) {
	tests := []struct {
		name   string
		client string
		now    string
		hour   int
		want   string
	}{
		{"before cutoff", variables.ZapCash, "2026-11-27 19:59:59", 20, policy.DecisionAllowed},
		{"at cutoff", variables.ZapCash, "2026-11-27 20:00:00", 20, policy.DecisionZapCashSendCutoff},
		{"after cutoff", variables.ZapCash, "2026-11-27 20:00:01", 20, policy.DecisionZapCashSendCutoff},
		{"disabled", variables.ZapCash, "2026-11-27 21:00:00", policy.ZapCashSendCutoffDisabledHour, policy.DecisionAllowed},
		{"non zapcash", variables.WeCredit, "2026-11-27 21:00:00", 20, policy.DecisionAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := policy.EvaluateZapCashSendCutoff(test.client, istTime(t, test.now), test.hour)
			if got.Code != test.want {
				t.Fatalf("decision=%s want %s", got.Code, test.want)
			}
		})
	}
}

func TestEvaluateZapCashSendCutoffUsesInjectedClock(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	restore := policy.SetClockForTest(func() time.Time {
		return time.Date(2026, 11, 27, 20, 0, 0, 0, loc)
	})
	defer restore()

	got := policy.EvaluateZapCashSendCutoff(variables.ZapCash, policy.Now(), 20)
	if got.Code != policy.DecisionZapCashSendCutoff {
		t.Fatalf("decision=%s want cutoff", got.Code)
	}
}
