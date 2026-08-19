package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
)

func TestKey(t *testing.T) {
	if got := ratelimit.Key("SINCH", "WeCredit"); got != "sinch:wecredit" {
		t.Fatalf("Key = %q", got)
	}
}

func TestParseOverrides(t *testing.T) {
	got := ratelimit.ParseOverrides("sinch:wecredit:100,pinnacle:80,times:default:25")
	if got["sinch:wecredit"] != 100 || got["pinnacle"] != 80 || got["times:default"] != 25 {
		t.Fatalf("unexpected overrides: %#v", got)
	}
}

func TestTokenBucketAllowsBurstThenWaits(t *testing.T) {
	ratelimit.ResetForTest()
	config.Configs.ProviderRPSDefault = "5"
	config.Configs.ProviderRPSOverrides = "test:unit:20"

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := ratelimit.WaitFor(ctx, "test:unit"); err != nil {
			t.Fatalf("WaitFor: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("burst should be near-instant, took %v", elapsed)
	}
}

func TestWaitForCancelled(t *testing.T) {
	ratelimit.ResetForTest()
	config.Configs.ProviderRPSDefault = "1"
	config.Configs.ProviderRPSOverrides = "slow:client:1"

	ctx, cancel := context.WithCancel(context.Background())
	_ = ratelimit.WaitFor(ctx, "slow:client") // consume burst token
	cancel()
	if err := ratelimit.WaitFor(ctx, "slow:client"); err == nil {
		t.Fatal("expected cancelled wait error")
	}
}
