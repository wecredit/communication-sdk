package config_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/ratelimit"
)

// Overlay behavior is exercised via ratelimit.TryApplyOverrides (same path as
// config.overlayProviderRPSAndWorkerAppConfig). Blank/invalid must not replace last-good.
func TestOverridesOverlayKeepsLastGoodOnInvalid(t *testing.T) {
	ratelimit.ResetForTest()
	caps := "times:wecredit:whatsapp:120,pinnacle:wecredit:whatsapp:40"
	if err := ratelimit.InitFromConfig("times:wecredit:whatsapp:100", caps, "10", "2"); err != nil {
		t.Fatal(err)
	}
	if err := ratelimit.TryApplyOverrides("times:wecredit:whatsapp:999", caps); err == nil {
		t.Fatal("expected invalid overlay to fail")
	}
	if got := ratelimit.LastGoodOverridesRaw(); got != "times:wecredit:whatsapp:100" {
		t.Fatalf("last-good = %q", got)
	}
}

func TestOverridesOverlayAppliesValid(t *testing.T) {
	ratelimit.ResetForTest()
	caps := "times:wecredit:whatsapp:120"
	if err := ratelimit.InitFromConfig("times:wecredit:whatsapp:80", caps, "10", "2"); err != nil {
		t.Fatal(err)
	}
	if err := ratelimit.TryApplyOverrides("times:wecredit:whatsapp:110", caps); err != nil {
		t.Fatal(err)
	}
	if got := ratelimit.ResolvedRPS("times:wecredit:whatsapp"); got != 110 {
		t.Fatalf("resolved = %v", got)
	}
}
