package smsCompliance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	pinnacle "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle"
	sinch "github.com/wecredit/communication-sdk/internal/channels/sms/sinch"
	times "github.com/wecredit/communication-sdk/internal/channels/sms/times"
	"github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	models "github.com/wecredit/communication-sdk/sdk/models"
	smspolicy "github.com/wecredit/communication-sdk/sdk/policy"
)

func TestEverySMSProviderBlocksImmediatelyBeforeHTTP(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 8, 23, 20, 0, 1, 0, loc)
	restoreClock := smspolicy.SetClockForTest(func() time.Time { return cutoff })
	t.Cleanup(restoreClock)

	previous := config.Configs
	t.Cleanup(func() { config.Configs = previous })
	config.Configs = models.Config{
		ProviderRPSDefault:     "50",
		SinchSmsApiUrl:         "http://127.0.0.1:1/sinch",
		SinchSmsApiUserName:    "user",
		SinchSmsApiPassword:    "password",
		SinchSmsApiAppID:       "app",
		SinchSmsApiSender:      "sender",
		TimesSmsApiUrl:         "http://127.0.0.1:1/times",
		TimesSmsApiSender:      "sender",
		PinnacleSmsApiUrl:      "http://127.0.0.1:1/pinnacle",
		PinnacleSmsAccessKey:   "secret",
		PinnacleSmsDltEntityId: "entity",
	}
	ratelimit.ResetForTest()

	request := extapimodels.SmsRequestBody{
		Client: "wecredit", Process: "campaign", Channel: "SMS", Mobile: "9876543210",
		TemplateText: "test", TemplateHeader: "WECRDT", DltTemplateId: 123,
		Source: "marketing", SourceRowId: 42, CampaignDate: "2026-08-23",
	}

	responses := map[string]extapimodels.SmsResponse{
		"sinch":    sinch.HitSinchSmsApi(request),
		"times":    times.HitTimesSmsApi(request),
		"pinnacle": pinnacle.HitPinnacleApi(request),
	}
	for provider, response := range responses {
		if response.Outcome != outcome.FailedFinal {
			t.Errorf("%s outcome = %s, want %s", provider, response.Outcome, outcome.FailedFinal)
		}
		if !strings.Contains(response.ResponseMessage, smspolicy.DecisionCutoff) {
			t.Errorf("%s response = %q, want cutoff code", provider, response.ResponseMessage)
		}
	}
}
