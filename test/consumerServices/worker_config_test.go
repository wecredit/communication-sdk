package consumerServices_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/config"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
)

func TestClientWorkerCountUsesDefaultAndOverrides(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:25,zapcash:5,creditsea:7"
	config.Configs.ConsumerChannelWorkerOverrides = ""

	for _, test := range []struct {
		client string
		want   int
	}{
		{client: "wecredit", want: 25},
		{client: "ZapCash", want: 5},
		{client: "creditsea", want: 7},
		{client: "trustfin", want: 5},
	} {
		if got := services.ClientWorkerCount(test.client); got != test.want {
			t.Fatalf("ClientWorkerCount(%q) = %d, want %d", test.client, got, test.want)
		}
	}
}

func TestClientChannelWorkerCountAndPoolKey(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:50"
	config.Configs.ConsumerChannelWorkerOverrides = "wecredit:sms:70,wecredit:whatsapp:140,zapcash:sms:20"

	if got := services.ClientChannelPoolKey("WeCredit", "SMS"); got != "wecredit|sms" {
		t.Fatalf("pool key = %q", got)
	}
	if got := services.ClientChannelWorkerCount("wecredit", "sms"); got != 70 {
		t.Fatalf("sms workers = %d", got)
	}
	if got := services.ClientChannelWorkerCount("wecredit", "whatsapp"); got != 140 {
		t.Fatalf("wa workers = %d", got)
	}
	// Channel miss falls back to client override.
	if got := services.ClientChannelWorkerCount("wecredit", "rcs"); got != 50 {
		t.Fatalf("fallback client workers = %d", got)
	}
	// Other client channel must not pick up wecredit values.
	if got := services.ClientChannelWorkerCount("zapcash", "sms"); got != 20 {
		t.Fatalf("zapcash sms = %d", got)
	}
	if got := services.ClientChannelWorkerCount("zapcash", "whatsapp"); got != 5 {
		t.Fatalf("zapcash wa default = %d want 5", got)
	}
}

func TestClientSharedWorkerCountAddsChannelBudgets(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:10"
	config.Configs.ConsumerChannelWorkerOverrides = "wecredit:sms:50,wecredit:whatsapp:50"

	if got := services.ClientSharedWorkerCount("WeCredit"); got != 100 {
		t.Fatalf("shared worker budget = %d, want 100", got)
	}
}

func TestClientSharedWorkerCountFallsBackToClientBudget(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:25"
	config.Configs.ConsumerChannelWorkerOverrides = ""

	if got := services.ClientSharedWorkerCount("wecredit"); got != 25 {
		t.Fatalf("shared worker fallback = %d, want 25", got)
	}
}

func TestClientChannelWorkerCountNoCrossClientLeak(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:50,creditsea:7"
	config.Configs.ConsumerChannelWorkerOverrides = "wecredit:sms:70,creditsea:sms:15"

	before := services.ClientChannelWorkerCount("creditsea", "sms")
	config.Configs.ConsumerChannelWorkerOverrides = "wecredit:sms:99,creditsea:sms:15"
	if got := services.ClientChannelWorkerCount("creditsea", "sms"); got != before {
		t.Fatalf("creditsea changed after wecredit bump: %d -> %d", before, got)
	}
	if got := services.ClientChannelWorkerCount("wecredit", "sms"); got != 99 {
		t.Fatalf("wecredit sms = %d", got)
	}
}

func TestClientChannelWorkerCountChannelSettings(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:25"
	config.Configs.ConsumerChannelWorkerOverrides = ""

	if got := services.ClientChannelWorkerCount("wecredit", "sms"); got != 25 {
		t.Fatalf("without channel setting, sms workers = %d, want existing client override 25", got)
	}

	config.Configs.SMSWorkers = "15"
	if got := services.ClientChannelWorkerCount("wecredit", "sms"); got != 15 {
		t.Fatalf("sms workers = %d, want 15", got)
	}
	if got := services.ClientChannelWorkerCount("wecredit", "whatsapp"); got != 25 {
		t.Fatalf("sms setting affected whatsapp: got %d, want client override 25", got)
	}

	config.Configs.WhatsAppWorkers = "35"
	if got := services.ClientChannelWorkerCount("wecredit", "whatsapp"); got != 35 {
		t.Fatalf("whatsapp workers = %d, want 35", got)
	}
	if got := services.ClientChannelWorkerCount("wecredit", "SMS"); got != 15 {
		t.Fatalf("whatsapp setting affected sms: got %d, want 15", got)
	}

	config.Configs.ConsumerChannelWorkerOverrides = "wecredit:sms:20"
	if got := services.ClientChannelWorkerCount("wecredit", "sms"); got != 20 {
		t.Fatalf("explicit channel override = %d, want 20", got)
	}
}

func TestClientChannelWorkerCountInvalidChannelSettingsFallThrough(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:25"
	config.Configs.ConsumerChannelWorkerOverrides = ""

	for _, raw := range []string{"", "invalid", "0", "-1"} {
		config.Configs.SMSWorkers = raw
		if got := services.ClientChannelWorkerCount("wecredit", "sms"); got != 25 {
			t.Fatalf("SMS_WORKERS=%q resolved to %d, want client override 25", raw, got)
		}
	}

	config.Configs.SMSWorkers = "501"
	if got := services.ClientChannelWorkerCount("wecredit", "sms"); got != 500 {
		t.Fatalf("SMS_WORKERS=501 resolved to %d, want capped 500", got)
	}
}
