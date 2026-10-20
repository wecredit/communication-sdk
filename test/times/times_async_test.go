package timesWhatsapp_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/config"
	timesWhatsapp "github.com/wecredit/communication-sdk/internal/channels/whatsapp/times"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func TestTimesWpModeSyncByDefault(t *testing.T) {
	mode, err := config.ResolveTimesWpMode("")
	if err != nil {
		t.Fatal(err)
	}
	if mode != config.TimesWpModeSync {
		t.Fatalf("mode = %q", mode)
	}
	url, err := config.TimesWpSendURL(mode, "https://sync.example/send", "")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://sync.example/send" {
		t.Fatalf("url = %q", url)
	}
	if config.TimesWpRateChannel(mode) != "whatsapp" {
		t.Fatalf("channel = %q", config.TimesWpRateChannel(mode))
	}
	if err := config.ValidateTimesWpMode(models.Config{TimesWpApiMode: "sync"}); err != nil {
		t.Fatal(err)
	}
}

func TestTimesWpModeAsyncRequiresURL(t *testing.T) {
	mode, err := config.ResolveTimesWpMode("ASYNC")
	if err != nil {
		t.Fatal(err)
	}
	if mode != config.TimesWpModeAsync {
		t.Fatalf("mode = %q", mode)
	}
	if _, err := config.TimesWpSendURL(mode, "https://sync.example/send", "  "); err == nil {
		t.Fatal("expected missing async URL error")
	}
	url, err := config.TimesWpSendURL(mode, "https://sync.example/send", " https://asyncmsg.integration.timespanel.in/wa/v1/messages/send ")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://asyncmsg.integration.timespanel.in/wa/v1/messages/send" {
		t.Fatalf("url = %q", url)
	}
	if config.TimesWpRateChannel(mode) != "whatsapp-async" {
		t.Fatalf("channel = %q", config.TimesWpRateChannel(mode))
	}
	if err := config.ValidateTimesWpMode(models.Config{TimesWpApiMode: "async"}); err == nil {
		t.Fatal("expected startup error when async URL is empty")
	}
}

func TestValidateTimesWpFallbackRequiresTemplateListBase(t *testing.T) {
	cfg := models.Config{
		TimesWpApiMode:                              "async",
		TimesWpAsyncApiUrl:                          "https://asyncmsg.integration.timespanel.in/wa/v1/messages/send",
		WhatsappTemplateSyncEnabled:                 "true",
		WhatsappTemplateSyncAllowSingleHostFallback: "true",
	}
	if err := config.ValidateTimesWpMode(cfg); err == nil {
		t.Fatal("expected template list base error")
	}
	cfg.WhatsappTemplateSyncAllowSingleHostFallback = "false"
	if err := config.ValidateTimesWpMode(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.WhatsappTemplateSyncAllowSingleHostFallback = "true"
	cfg.TimesWpApiUrl = "https://sync.example/wa/v1/messages/send"
	if err := config.ValidateTimesWpMode(cfg); err != nil {
		t.Fatal(err)
	}
	if got := config.TimesTemplateListBaseURL("", cfg.TimesWpApiUrl); got != "https://sync.example" {
		t.Fatalf("base = %q", got)
	}
	if got := config.TimesTemplateListBaseURL("https://panel.example/", ""); got != "https://panel.example" {
		t.Fatalf("base = %q", got)
	}
}

func TestTimesWpModeRejectsUnknown(t *testing.T) {
	if _, err := config.ResolveTimesWpMode("both"); err == nil {
		t.Fatal("expected unknown mode error")
	}
}

func TestClassifyTimesAsyncAccepted(t *testing.T) {
	raw := []byte(`{"status":"success","ct_trace_id":"asyncmsg:wa:v1:abc","message":"accepted"}`)
	got, err := timesWhatsapp.ClassifyTimesAsyncSend(200, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsSent {
		t.Fatal("expected IsSent")
	}
	if got.TransactionId != "asyncmsg:wa:v1:abc" {
		t.Fatalf("TransactionId = %q", got.TransactionId)
	}
	if !strings.Contains(got.ResponseMessage, "accepted") || !strings.Contains(got.ResponseMessage, "asyncmsg:wa:v1:abc") {
		t.Fatalf("ResponseMessage = %q", got.ResponseMessage)
	}
}

func TestClassifyTimesAsyncMissingTraceID(t *testing.T) {
	raw := []byte(`{"status":"success","message":"accepted"}`)
	got, err := timesWhatsapp.ClassifyTimesAsyncSend(200, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSent || got.TransactionId != "" {
		t.Fatalf("got IsSent=%v TransactionId=%q", got.IsSent, got.TransactionId)
	}
	if !strings.Contains(got.ResponseMessage, "ct_trace_id") {
		t.Fatalf("ResponseMessage = %q", got.ResponseMessage)
	}
}

func TestClassifyTimesAsyncInvalidRequest(t *testing.T) {
	raw := []byte(`{"ct_trace_id":"asyncmsg:wa:v1:bad","status":"failure","message":"Invalid request","code":400}`)
	got, err := timesWhatsapp.ClassifyTimesAsyncSend(200, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSent {
		t.Fatal("invalid request must not be sent")
	}
	if !strings.Contains(got.ResponseMessage, "Invalid request") || !strings.Contains(got.ResponseMessage, "400") {
		t.Fatalf("ResponseMessage = %q", got.ResponseMessage)
	}
}

func TestClassifyTimesAsyncUnauthorized(t *testing.T) {
	raw := []byte(`{"ct_trace_id":"asyncmsg:wa:v1:nope","status":"failure","message":"Unauthorized","code":403}`)
	got, err := timesWhatsapp.ClassifyTimesAsyncSend(403, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSent {
		t.Fatal("unauthorized must not be sent")
	}
	if !strings.Contains(got.ResponseMessage, "Unauthorized") || !strings.Contains(got.ResponseMessage, "403") {
		t.Fatalf("ResponseMessage = %q", got.ResponseMessage)
	}
}

func TestClassifyTimesAsyncRateLimited(t *testing.T) {
	raw := []byte(`{"status":"failure","message":"Too many requests","code":429}`)
	got, err := timesWhatsapp.ClassifyTimesAsyncSend(429, raw)
	if err == nil {
		t.Fatal("expected retryable error")
	}
	if got.IsSent {
		t.Fatal("429 must not be sent")
	}
}

func TestClassifyTimesAsyncServiceUnavailable(t *testing.T) {
	raw := []byte(`{"status":"failure","message":"internal service error"}`)
	got, err := timesWhatsapp.ClassifyTimesAsyncSend(503, raw)
	if err == nil {
		t.Fatal("expected transport error")
	}
	if got.IsSent {
		t.Fatal("5xx must not be sent")
	}
}

func TestClassifyTimesAsyncNonJSON(t *testing.T) {
	_, err := timesWhatsapp.ClassifyTimesAsyncSend(200, []byte("not-json"))
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestSyncBodyRejectsAsyncStatusString(t *testing.T) {
	raw := []byte(`{"status":"success","message":"accepted","ct_trace_id":"asyncmsg:wa:v1:abc"}`)
	_, err := timesWhatsapp.ApplyTimesSyncBody(raw)
	if err == nil {
		t.Fatal("sync parser must reject a string status")
	}
}

func TestSyncBodyKeepsWamid(t *testing.T) {
	raw := []byte(`{"status":true,"message":"ok","message_id":"m1","res_json":{"messages":[{"id":"wamid.1"}]}}`)
	got, err := timesWhatsapp.ApplyTimesSyncBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsSent || got.TransactionId != "wamid.1" {
		t.Fatalf("IsSent=%v TransactionId=%q message=%q", got.IsSent, got.TransactionId, got.ResponseMessage)
	}
}

func TestAsyncRateCapDoesNotRaiseSyncCap(t *testing.T) {
	// Async caps stay optional. Production supplies approved caps explicitly via env.
	caps := "times:wecredit:whatsapp:120,times:wecredit:whatsapp-async:200"
	if err := ratelimit.ValidateOverridesAgainstCaps("times:wecredit:whatsapp-async:200", caps); err != nil {
		t.Fatal(err)
	}
	if err := ratelimit.ValidateOverridesAgainstCaps("times:wecredit:whatsapp:400", caps); err == nil {
		t.Fatal("sync override of 400 must be rejected by the sync cap")
	}
	field, ok := reflect.TypeOf(models.Config{}).FieldByName("ProviderRPSApprovedCaps")
	if !ok {
		t.Fatal("missing ProviderRPSApprovedCaps")
	}
	defaultCaps := field.Tag.Get("default")
	if err := ratelimit.ValidateOverridesAgainstCaps("times:wecredit:whatsapp:120", defaultCaps); err != nil {
		t.Fatalf("sync default cap missing: %v", err)
	}
	if err := ratelimit.ValidateOverridesAgainstCaps("times:wecredit:whatsapp-async:200", defaultCaps); err == nil {
		t.Fatal("dormant async cap must not be part of the approved defaults")
	}
}
