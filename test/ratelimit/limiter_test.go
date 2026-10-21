package ratelimit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/ratelimit"
)

const testCaps = "sinch:wecredit:whatsapp:130,sinch:wecredit:sms:83,sinch:zapcash:sms:200,test:unit:whatsapp:20,times:wecredit:whatsapp:120,pinnacle:wecredit:whatsapp:40"

func TestKeyWithChannel(t *testing.T) {
	if got := ratelimit.Key("SINCH", "WeCredit"); got != "sinch:wecredit" {
		t.Fatalf("Key = %q", got)
	}
	if got := ratelimit.KeyWithChannel("SINCH", "WeCredit", "WHATSAPP"); got != "sinch:wecredit:whatsapp" {
		t.Fatalf("KeyWithChannel = %q", got)
	}
}

func TestResolvedRPSFallbackChainNoCrossClientLeak(t *testing.T) {
	ratelimit.ResetForTest()
	overrides := "sinch:wecredit:whatsapp:120,sinch:wecredit:sms:80,sinch:zapcash:sms:150"
	if err := ratelimit.InitFromConfig(overrides, testCaps, "50", "2"); err != nil {
		t.Fatal(err)
	}

	if got := ratelimit.ResolvedRPS("sinch:wecredit:whatsapp"); got != 120 {
		t.Fatalf("wecredit wa = %v, want 120", got)
	}
	if got := ratelimit.ResolvedRPS("sinch:wecredit:sms"); got != 80 {
		t.Fatalf("wecredit sms = %v, want 80", got)
	}
	if got := ratelimit.ResolvedRPS("sinch:zapcash:sms"); got != 150 {
		t.Fatalf("zapcash sms = %v, want 150", got)
	}

	if err := ratelimit.TryApplyOverrides(
		"sinch:wecredit:whatsapp:120,sinch:wecredit:sms:80,sinch:zapcash:sms:180",
		testCaps,
	); err != nil {
		t.Fatal(err)
	}
	if got := ratelimit.ResolvedRPS("sinch:wecredit:sms"); got != 80 {
		t.Fatalf("cross-client leak: wecredit sms became %v", got)
	}
	if got := ratelimit.ResolvedRPS("sinch:zapcash:sms"); got != 180 {
		t.Fatalf("zapcash sms = %v, want 180", got)
	}
}

func TestValidateOverridesAgainstCapsRejectsOverCap(t *testing.T) {
	err := ratelimit.ValidateOverridesAgainstCaps("sinch:wecredit:whatsapp:200", "sinch:wecredit:whatsapp:130,sinch:wecredit:sms:83")
	if err == nil || !strings.Contains(err.Error(), "provider_rps_overrides_exceed_cap") {
		t.Fatalf("want exceed_cap error, got %v", err)
	}
}

func TestValidateOverridesAgainstCapsRejectsMalformedEntry(t *testing.T) {
	err := ratelimit.ValidateOverridesAgainstCaps("sinch:wecredit:sms:not-a-number", testCaps)
	if err == nil || !strings.Contains(err.Error(), "invalid provider rate limit overrides entry") {
		t.Fatalf("want malformed override error, got %v", err)
	}
}

func TestValidateOverridesAgainstCapsRejectsNonFiniteRate(t *testing.T) {
	for _, raw := range []string{"sinch:wecredit:sms:NaN", "sinch:wecredit:sms:+Inf"} {
		if err := ratelimit.ValidateOverridesAgainstCaps(raw, testCaps); err == nil {
			t.Fatalf("expected non-finite rate %q to be rejected", raw)
		}
	}
}

func TestTryApplyOverridesKeepsLastGoodOnRefuse(t *testing.T) {
	ratelimit.ResetForTest()
	caps := "sinch:wecredit:whatsapp:130"
	if err := ratelimit.InitFromConfig("sinch:wecredit:whatsapp:100", caps, "50", "2"); err != nil {
		t.Fatal(err)
	}

	err := ratelimit.TryApplyOverrides("sinch:wecredit:whatsapp:999", caps)
	if err == nil {
		t.Fatal("expected refuse")
	}
	if got := ratelimit.LastGoodOverridesRaw(); got != "sinch:wecredit:whatsapp:100" {
		t.Fatalf("last-good = %q", got)
	}
	if got := ratelimit.ResolvedRPS("sinch:wecredit:whatsapp"); got != 100 {
		t.Fatalf("resolved after refuse = %v", got)
	}
}

func TestInitFromConfigFailFast(t *testing.T) {
	ratelimit.ResetForTest()
	if err := ratelimit.InitFromConfig("sinch:wecredit:sms:200", "sinch:wecredit:sms:83", "50", "2"); err == nil {
		t.Fatal("expected fail-fast")
	}
}

func TestWaitForUsesChannelKey(t *testing.T) {
	ratelimit.ResetForTest()
	if err := ratelimit.InitFromConfig("test:unit:whatsapp:20", testCaps, "5", "2"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	key := ratelimit.KeyWithChannel("test", "unit", "whatsapp")
	for i := 0; i < 5; i++ {
		if err := ratelimit.WaitFor(ctx, key); err != nil {
			t.Fatalf("wait %d: %v", i, err)
		}
	}
}

func TestLocalFallbackRPSDividesByTaskSlots(t *testing.T) {
	ratelimit.ResetForTest()
	if err := ratelimit.InitFromConfig("times:wecredit:whatsapp:120", testCaps, "10", "2"); err != nil {
		t.Fatal(err)
	}
	if got := ratelimit.LocalFallbackRPS("times:wecredit:whatsapp"); got != 60 {
		t.Fatalf("fallback = %v, want 60", got)
	}
	if got := ratelimit.TaskSlots(); got != 2 {
		t.Fatalf("task slots = %d, want 2", got)
	}
}

func TestWaitForUsesTaskLocalFallbackWhenRedisIsUnavailable(t *testing.T) {
	ratelimit.ResetForTest()
	if err := ratelimit.InitFromConfig("times:wecredit:whatsapp:4", "times:wecredit:whatsapp:4", "5", "2"); err != nil {
		t.Fatal(err)
	}
	key := ratelimit.KeyWithChannel("times", "wecredit", "whatsapp")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	// No Redis is configured, so the local bucket must use 4 / 2 = 2 RPS.
	if err := ratelimit.WaitFor(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if err := ratelimit.WaitFor(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if err := ratelimit.WaitFor(ctx, key); err == nil {
		t.Fatal("third immediate acquire should wait after the local two-token burst")
	}
}
