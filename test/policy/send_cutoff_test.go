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
		{"21", 21},
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

func TestParseZapCashSendOpenHourIST(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", policy.DefaultZapCashSendOpenHourIST},
		{"8", 8},
		{"9", 9},
		{"0", 0},
		{"-1", policy.ZapCashSendOpenDisabledHour},
		{"24", policy.DefaultZapCashSendOpenHourIST},
		{"abc", policy.DefaultZapCashSendOpenHourIST},
	}
	for _, test := range tests {
		if got := policy.ParseZapCashSendOpenHourIST(test.raw); got != test.want {
			t.Fatalf("ParseZapCashSendOpenHourIST(%q)=%d want %d", test.raw, got, test.want)
		}
	}
}

func TestEvaluateZapCashSendCutoff(t *testing.T) {
	tests := []struct {
		name   string
		client string
		now    string
		open   int
		hour   int
		want   string
	}{
		{"before cutoff", variables.ZapCash, "2026-11-27 20:59:59", 8, 21, policy.DecisionAllowed},
		{"at cutoff", variables.ZapCash, "2026-11-27 21:00:00", 8, 21, policy.DecisionZapCashSendCutoff},
		{"after cutoff", variables.ZapCash, "2026-11-27 21:00:01", 8, 21, policy.DecisionZapCashSendCutoff},
		{"overnight leftover", variables.ZapCash, "2026-11-28 00:22:00", 8, 21, policy.DecisionZapCashSendCutoff},
		{"before open", variables.ZapCash, "2026-11-28 07:59:59", 8, 21, policy.DecisionZapCashSendCutoff},
		{"at open", variables.ZapCash, "2026-11-28 08:00:00", 8, 21, policy.DecisionAllowed},
		{"open disabled overnight allows", variables.ZapCash, "2026-11-28 00:22:00", policy.ZapCashSendOpenDisabledHour, 21, policy.DecisionAllowed},
		{"disabled", variables.ZapCash, "2026-11-27 21:00:00", 8, policy.ZapCashSendCutoffDisabledHour, policy.DecisionAllowed},
		{"non zapcash", variables.WeCredit, "2026-11-27 21:00:00", 8, 21, policy.DecisionAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := policy.EvaluateZapCashSendCutoff(test.client, istTime(t, test.now), test.open, test.hour)
			if got.Code != test.want {
				t.Fatalf("decision=%s want %s msg=%q", got.Code, test.want, got.Message)
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
		return time.Date(2026, 11, 27, 21, 0, 0, 0, loc)
	})
	defer restore()

	got := policy.EvaluateZapCashSendCutoff(variables.ZapCash, policy.Now(), 8, 21)
	if got.Code != policy.DecisionZapCashSendCutoff {
		t.Fatalf("decision=%s want cutoff", got.Code)
	}
}
