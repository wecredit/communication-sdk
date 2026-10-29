package persistence_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/models/redisModels"
	"github.com/wecredit/communication-sdk/internal/persistence"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

type fakeProducerRedis struct {
	length  int64
	xaddID  string
	xaddErr error
	xadds   int
}

func (f *fakeProducerRedis) XLen(context.Context, string) *redislib.IntCmd {
	return redislib.NewIntResult(f.length, nil)
}

func (f *fakeProducerRedis) XAdd(context.Context, *redislib.XAddArgs) *redislib.StringCmd {
	f.xadds++
	return redislib.NewStringResult(f.xaddID, f.xaddErr)
}

func TestAsyncEnabledRequiresFlagAndBudget(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.CommAsyncWriteChannels = "wecredit:sms"
	config.Configs.CommWriteStreamBudgetOverrides = "wecredit:sms:100"
	if !persistence.AsyncEnabled("WECREDIT", "SMS") {
		t.Fatal("expected supported channel to be enabled")
	}
	config.Configs.CommWriteStreamBudgetOverrides = ""
	if persistence.AsyncEnabled("wecredit", "sms") {
		t.Fatal("async write must stay off without a verified stream budget")
	}
}

func TestMarketingSMSRecordRequiresSealedHitAt(t *testing.T) {
	_, err := persistence.MarketingSMSRecord(sdkModels.CommApiRequestBody{SourceRowId: 42}, nil, redisModels.MobileChannelRedisData{TransactionId: "provider-1"})
	if !errors.Is(err, persistence.ErrMissingHitAt) {
		t.Fatalf("error = %v, want ErrMissingHitAt", err)
	}
}

func TestMarketingSMSRecordUsesTerminalProviderIdentity(t *testing.T) {
	hitAt := time.Date(2026, 10, 28, 10, 30, 0, 0, time.UTC)
	record, err := persistence.MarketingSMSRecord(sdkModels.CommApiRequestBody{
		Source: "marketing", SourceRowId: 42, Client: "wecredit", Channel: "SMS",
		EventId: "event-42", Vendor: "SINCH", ProcessName: "CREDITT",
	}, nil, redisModels.MobileChannelRedisData{
		TransactionId: "txn-42", ProviderMessageID: "provider-42",
		Outcome: "submitted", HitAt: hitAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.TransactionID != "provider-42" || record.Outcome != "SENT" || !record.HitAt.Equal(hitAt) {
		t.Fatalf("record = %+v", record)
	}
}

func TestProducerPausesAtEightyPercentAndResumesAtSixty(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.CommWriteStreamBudgetOverrides = "wecredit:sms:100"
	fake := &fakeProducerRedis{length: 80, xaddID: "1-0"}
	producer := persistence.NewProducer(fake)
	record := persistence.Record{Client: "wecredit", Channel: "sms", Mapper: persistence.MapperMarketingSMS, HitAt: time.Now()}
	if _, err := producer.Enqueue(context.Background(), record); !errors.Is(err, persistence.ErrStreamHighWater) {
		t.Fatalf("error = %v, want high-water pause", err)
	}
	if fake.xadds != 0 {
		t.Fatal("paused producer must not XADD")
	}
	fake.length = 61
	if _, err := producer.Enqueue(context.Background(), record); !errors.Is(err, persistence.ErrStreamHighWater) {
		t.Fatalf("61%% must remain paused, error=%v", err)
	}
	fake.length = 60
	if _, err := producer.Enqueue(context.Background(), record); err != nil {
		t.Fatalf("60%% must resume: %v", err)
	}
	if fake.xadds != 1 {
		t.Fatalf("XADD count = %d, want 1", fake.xadds)
	}
}

func TestProducerReturnsXAddFailure(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.CommWriteStreamBudgetOverrides = "wecredit:whatsapp:100"
	fake := &fakeProducerRedis{length: 0, xaddErr: errors.New("redis full")}
	producer := persistence.NewProducer(fake)
	_, err := producer.Enqueue(context.Background(), persistence.Record{
		Client: "wecredit", Channel: "whatsapp", Mapper: persistence.MapperMarketingWhatsapp, HitAt: time.Now(),
	})
	if err == nil {
		t.Fatal("expected XADD failure")
	}
}

func TestStreamAndPoisonKeysShareRedisClusterSlot(t *testing.T) {
	stream := persistence.StreamKey("WeCredit", "SMS")
	poison := persistence.PoisonStreamKey("wecredit:sms")
	if stream != "commwrite:{wecredit:sms}:write" {
		t.Fatalf("stream key = %q", stream)
	}
	if poison != "commwrite:{wecredit:sms}:poison" {
		t.Fatalf("poison key = %q", poison)
	}
}

func TestMarketingSMSRecordUsesTrackingFieldLimitsAndRedaction(t *testing.T) {
	hitAt := time.Date(2026, 10, 29, 10, 30, 0, 0, time.UTC)
	record, err := persistence.MarketingSMSRecord(sdkModels.CommApiRequestBody{
		Source: "marketing", SourceRowId: 42, Client: "wecredit", Channel: "SMS",
		Vendor: strings.Repeat("V", 40), EventId: "event-42",
	}, nil, redisModels.MobileChannelRedisData{
		Outcome: "rejected", ErrorMessage: "failed for 919876543210", HitAt: hitAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Vendor) != 30 {
		t.Fatalf("vendor length = %d, want 30", len(record.Vendor))
	}
	if strings.Contains(record.ErrorMessage, "919876543210") || !strings.Contains(record.ErrorMessage, "[REDACTED]") {
		t.Fatalf("error message was not redacted: %q", record.ErrorMessage)
	}
}
