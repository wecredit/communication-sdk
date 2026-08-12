package channelHelper

import (
	"testing"

	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestGenerateRedisKeyForRequestUsesEventId(t *testing.T) {
	key := GenerateRedisKeyForRequest(sdkModels.CommApiRequestBody{
		Mobile:            "7014850582",
		Channel:           "SMS",
		Stage:             0,
		CommId:            "", // CommId generated later as WC-...
		EventId:           "marketing-10",
		ProcessName:       "fatakpay",
		TemplateReference: "1707177200722881790",
	})
	if key != "marketing-10" {
		t.Fatalf("got %q, want marketing-10", key)
	}

	other := GenerateRedisKeyForRequest(sdkModels.CommApiRequestBody{
		Mobile:            "7014850582",
		Channel:           "SMS",
		Stage:             0,
		EventId:           "marketing-9",
		ProcessName:       "truebalance",
		TemplateReference: "1707177234912222297",
	})
	if other != "marketing-9" {
		t.Fatalf("got %q, want marketing-9", other)
	}
	if key == other {
		t.Fatal("distinct marketing rows must not share a Redis key")
	}
}

func TestGenerateRedisKeyForRequestIgnoresCommId(t *testing.T) {
	key := GenerateRedisKeyForRequest(sdkModels.CommApiRequestBody{
		Mobile:  "7014850582",
		Channel: "SMS",
		Stage:   1.02,
		CommId:  "WC-WECREDIT-already-set",
	})
	want := "7014850582_SMS_1"
	if key != want {
		t.Fatalf("got %q, want %q (CommId must not drive Redis key)", key, want)
	}
}

func TestGenerateRedisKeyForRequestTemplateReferenceWithoutEventId(t *testing.T) {
	key := GenerateRedisKeyForRequest(sdkModels.CommApiRequestBody{
		Mobile:            "7014850582",
		Channel:           "sms",
		Stage:             0,
		ProcessName:       "fatakpay",
		TemplateReference: "TPL_A",
	})
	want := "7014850582_SMS_fatakpay_tpl_a"
	if key != want {
		t.Fatalf("got %q, want %q", key, want)
	}
}

func TestGenerateRedisKeyForRequestLegacyStage(t *testing.T) {
	key := GenerateRedisKeyForRequest(sdkModels.CommApiRequestBody{
		Mobile:  "7014850582",
		Channel: "SMS",
		Stage:   1.02,
		Client:  "creditsea",
	})
	want := "7014850582_SMS_1"
	if key != want {
		t.Fatalf("got %q, want %q", key, want)
	}
}
