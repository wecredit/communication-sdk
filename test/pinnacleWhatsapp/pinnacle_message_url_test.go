package pinnacleWhatsapp_test

import (
	"testing"

	pinnacleWhatsapp "github.com/wecredit/communication-sdk/internal/channels/whatsapp/pinnacle"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func TestResolvePinnacleMessageURLPrefersBaseAppId(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })

	config.Configs.PinnacleWhatsappBaseUrl = "https://partnersv1.pinbot.ai/v3"
	config.Configs.PinnacleWhatsappMessageApiUrl = "https://partnersv1.pinbot.ai/v3/OTHER/messages"
	config.Configs.PinnacleZapcashWhatsappMessageApiUrl = "https://zap.example/messages"

	got := pinnacleWhatsapp.ResolvePinnacleMessageURL("wecredit", "1000221073181482")
	want := "https://partnersv1.pinbot.ai/v3/1000221073181482/messages"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolvePinnacleMessageURLFallsBackToFullOverride(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })

	config.Configs.PinnacleWhatsappBaseUrl = ""
	config.Configs.PinnacleWhatsappTemplateListBaseUrl = ""
	config.Configs.PinnacleWhatsappMessageApiUrl = "https://partnersv1.pinbot.ai/v3/FIXED/messages"

	got := pinnacleWhatsapp.ResolvePinnacleMessageURL("wecredit", "")
	want := "https://partnersv1.pinbot.ai/v3/FIXED/messages"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolvePinnacleMessageURLZapCash(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })

	config.Configs.PinnacleZapcashWhatsappMessageApiUrl = "https://zap.example/v3/Z/messages"
	config.Configs.PinnacleWhatsappBaseUrl = "https://partnersv1.pinbot.ai/v3"

	got := pinnacleWhatsapp.ResolvePinnacleMessageURL(variables.ZapCash, "ignored")
	want := "https://zap.example/v3/Z/messages"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
