package config_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func TestAppConfigOverlayAppliesRPSAndWorkerSettings(t *testing.T) {
	candidate := models.Config{
		ProviderRPSTaskSlots:           "2",
		ConsumerDefaultClientWorkers:   "5",
		ConsumerChannelWorkerOverrides: "wecredit:sms:60",
		ConsumerClientBufferSize:       "200",
	}
	config.ApplyAppConfigValues(&candidate, map[string]string{
		"PROVIDER_RPS_TASK_SLOTS":           "4",
		"CONSUMER_DEFAULT_CLIENT_WORKERS":   "12",
		"CONSUMER_CLIENT_WORKER_OVERRIDES":  "wecredit:20",
		"CONSUMER_CHANNEL_WORKER_OVERRIDES": "wecredit:sms:30",
		"CONSUMER_CLIENT_BUFFER_SIZE":       "400",
		"SMS_WORKERS":                       "25",
		"WHATSAPP_WORKERS":                  "35",
	})

	if candidate.ProviderRPSTaskSlots != "4" || candidate.ConsumerDefaultClientWorkers != "12" || candidate.ConsumerClientWorkerOverrides != "wecredit:20" || candidate.ConsumerChannelWorkerOverrides != "wecredit:sms:30" || candidate.ConsumerClientBufferSize != "400" || candidate.SMSWorkers != "25" || candidate.WhatsAppWorkers != "35" {
		t.Fatalf("AppConfig overlay did not apply settings: %+v", candidate)
	}
}

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
