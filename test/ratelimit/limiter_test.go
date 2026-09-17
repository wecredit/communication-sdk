package ratelimit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/ratelimit"
)

const testCaps = "sinch:wecredit:whatsapp:130,sinch:wecredit:sms:83,sinch:zapcash:sms:200,test:unit:whatsapp:20"

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
	if err := ratelimit.InitFromConfig(overrides, testCaps, "50"); err != nil {
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

func TestTryApplyOverridesKeepsLastGoodOnRefuse(t *testing.T) {
	ratelimit.ResetForTest()
	caps := "sinch:wecredit:whatsapp:130"
	if err := ratelimit.InitFromConfig("sinch:wecredit:whatsapp:100", caps, "50"); err != nil {
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
	if err := ratelimit.InitFromConfig("sinch:wecredit:sms:200", "sinch:wecredit:sms:83", "50"); err == nil {
		t.Fatal("expected fail-fast")
	}
}

func TestWaitForUsesChannelKey(t *testing.T) {
	ratelimit.ResetForTest()
	if err := ratelimit.InitFromConfig("test:unit:whatsapp:20", testCaps, "5"); err != nil {
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
