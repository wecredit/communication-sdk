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
