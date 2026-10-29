package config_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func TestAppConfigOverlayAppliesOnlyFrequentlyChangedSettings(t *testing.T) {
	candidate := models.Config{
		ProviderRPSTaskSlots:           "2",
		ConsumerDefaultClientWorkers:   "5",
		ConsumerChannelWorkerOverrides: "wecredit:sms:60",
		ConsumerClientBufferSize:       "200",
		GoMaxProcs:                     "1",
	}
	config.ApplyAppConfigValues(&candidate, map[string]string{
		"PROVIDER_RPS_OVERRIDES":             "times:wecredit:sms:110",
		"PROVIDER_RPS_TASK_SLOTS":            "4",
		"CONSUMER_DEFAULT_CLIENT_WORKERS":    "12",
		"CONSUMER_CLIENT_WORKER_OVERRIDES":   "wecredit:20",
		"CONSUMER_CHANNEL_WORKER_OVERRIDES":  "wecredit:sms:30",
		"CONSUMER_CLIENT_BUFFER_SIZE":        "400",
		"SMS_WORKERS":                        "25",
		"WHATSAPP_WORKERS":                   "35",
		"GOMAXPROCS":                         "2",
		"LOG_LEVEL":                          "debug",
		"COMM_ASYNC_WRITE_CHANNELS":          "wecredit:sms",
		"COMM_WRITE_STREAM_BUDGET_OVERRIDES": "wecredit:sms:100000",
	})

	if candidate.ProviderRPSOverrides != "times:wecredit:sms:110" || candidate.ConsumerClientWorkerOverrides != "wecredit:20" || candidate.ConsumerChannelWorkerOverrides != "wecredit:sms:30" || candidate.LogLevel != "debug" || candidate.CommAsyncWriteChannels != "wecredit:sms" || candidate.CommWriteStreamBudgetOverrides != "wecredit:sms:100000" {
		t.Fatalf("AppConfig overlay did not apply settings: %+v", candidate)
	}
	if candidate.ProviderRPSTaskSlots != "2" || candidate.ConsumerDefaultClientWorkers != "5" || candidate.ConsumerClientBufferSize != "200" || candidate.SMSWorkers != "" || candidate.WhatsAppWorkers != "" || candidate.GoMaxProcs != "1" {
		t.Fatalf("ENV-owned settings were overwritten by AppConfig: %+v", candidate)
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
