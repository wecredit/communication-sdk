package persistence_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/models/redisModels"
	"github.com/wecredit/communication-sdk/internal/persistence"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

type fakeProducerRedis struct {
	mu      sync.Mutex
	length  int64
	xaddID  string
	xaddErr error
	xadds   int
}

type fakeRedisSafetyClient struct {
	info        string
	infoErr     error
	policy      map[string]string
	configErr   error
	configCalls int
}

type fakeBusinessLockRedis struct {
	mu     sync.Mutex
	owners map[string]string
}

func (f *fakeBusinessLockRedis) SetNX(_ context.Context, key string, value interface{}, _ time.Duration) *redislib.BoolCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.owners == nil {
		f.owners = make(map[string]string)
	}
	if _, exists := f.owners[key]; exists {
		return redislib.NewBoolResult(false, nil)
	}
	f.owners[key] = fmt.Sprint(value)
	return redislib.NewBoolResult(true, nil)
}

func (f *fakeBusinessLockRedis) Eval(_ context.Context, script string, keys []string, args ...interface{}) *redislib.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := keys[0]
	token := fmt.Sprint(args[0])
	if f.owners[key] != token {
		return redislib.NewCmdResult(int64(0), nil)
	}
	if strings.Contains(script, "DEL") {
		delete(f.owners, key)
	}
	return redislib.NewCmdResult(int64(1), nil)
}

func (f *fakeRedisSafetyClient) Info(context.Context, ...string) *redislib.StringCmd {
	return redislib.NewStringResult(f.info, f.infoErr)
}

func (f *fakeRedisSafetyClient) ConfigGet(context.Context, string) *redislib.MapStringStringCmd {
	f.configCalls++
	return redislib.NewMapStringStringResult(f.policy, f.configErr)
}

func (f *fakeProducerRedis) XLen(context.Context, string) *redislib.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return redislib.NewIntResult(f.length, nil)
}

func (f *fakeProducerRedis) Eval(_ context.Context, _ string, _ []string, args ...interface{}) *redislib.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	limit, _ := args[0].(int64)
	if f.length >= limit {
		return redislib.NewCmdResult(nil, redislib.Nil)
	}
	f.xadds++
	if f.xaddErr == nil {
		f.length++
	}
	return redislib.NewCmdResult(f.xaddID, f.xaddErr)
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

func TestVerifyRedisSafetyAcceptsElastiCacheServerlessWithoutConfigGet(t *testing.T) {
	client := &fakeRedisSafetyClient{
		info:      "# Server\r\nvalkey_version:8.1.0\r\n",
		configErr: errors.New("ERR unknown command 'config'"),
	}
	err := persistence.VerifyRedisSafety(
		context.Background(),
		client,
		"nurture-engine-example.serverless.aps1.cache.amazonaws.com:6379",
	)
	if err != nil {
		t.Fatalf("ElastiCache Serverless safety check failed: %v", err)
	}
	if client.configCalls != 0 {
		t.Fatalf("CONFIG GET calls = %d, want 0 for ElastiCache Serverless", client.configCalls)
	}
}

func TestVerifyRedisSafetyKeepsStrictPolicyCheckForOtherRedis(t *testing.T) {
	for _, test := range []struct {
		name    string
		policy  string
		wantErr bool
	}{
		{name: "noeviction", policy: "noeviction"},
		{name: "volatile lru", policy: "volatile-lru", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeRedisSafetyClient{
				info:   "# Server\r\nredis_version:7.2.0\r\n",
				policy: map[string]string{"maxmemory-policy": test.policy},
			}
			err := persistence.VerifyRedisSafety(context.Background(), client, "localhost:6379")
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr=%t", err, test.wantErr)
			}
			if client.configCalls != 1 {
				t.Fatalf("CONFIG GET calls = %d, want 1", client.configCalls)
			}
		})
	}
}

func TestVerifyRedisSafetyRejectsOldServerlessEngine(t *testing.T) {
	client := &fakeRedisSafetyClient{info: "# Server\r\nredis_version:6.0.0\r\n"}
	err := persistence.VerifyRedisSafety(
		context.Background(),
		client,
		"cache.serverless.aps1.cache.amazonaws.com:6379",
	)
	if err == nil || !strings.Contains(err.Error(), "require >=6.2") {
		t.Fatalf("error = %v, want version rejection", err)
	}
	if client.configCalls != 0 {
		t.Fatalf("CONFIG GET calls = %d, want 0 after version rejection", client.configCalls)
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

func TestMarketingSMSRecordNormalizesTerminalOutcomes(t *testing.T) {
	hitAt := time.Date(2026, 10, 29, 10, 30, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for input, want := range map[string]string{
		"FAILED_FINAL": "FAILED", "SKIPPED": "FAILED", "ALREADY_DONE": "SENT",
	} {
		t.Run(input, func(t *testing.T) {
			record, err := persistence.MarketingSMSRecord(sdkModels.CommApiRequestBody{
				Source: "marketing", SourceRowId: 42, Client: "wecredit", Channel: "SMS",
			}, nil, redisModels.MobileChannelRedisData{Outcome: input, HitAt: hitAt})
			if err != nil {
				t.Fatal(err)
			}
			if record.Outcome != want {
				t.Fatalf("outcome = %q, want %q", record.Outcome, want)
			}
		})
	}
}

func TestMarketingWhatsappRecordPreservesRawBoundaryWhitespace(t *testing.T) {
	hitAt := time.Date(2026, 10, 29, 10, 30, 0, 0, time.UTC)
	record, err := persistence.MarketingWhatsappRecord(sdkModels.CommApiRequestBody{
		SourceRowId: 42, Client: "wecredit", Channel: "WHATSAPP",
	}, map[string]interface{}{
		"RawPayload": "  payload\n", "RawResponse": "\tresponse  ",
	}, redisModels.MobileChannelRedisData{Outcome: "submitted", HitAt: hitAt.Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if record.RawPayload != "  payload\n" || record.RawResponse != "\tresponse  " {
		t.Fatalf("raw values changed: payload=%q response=%q", record.RawPayload, record.RawResponse)
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

func TestProducerAtomicallyCapsConcurrentEnqueue(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.CommWriteStreamBudgetOverrides = "wecredit:sms:10"
	fake := &fakeProducerRedis{xaddID: "1-0"}
	producer := persistence.NewProducer(fake)
	record := persistence.Record{Client: "wecredit", Channel: "sms", HitAt: time.Now()}

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := producer.Enqueue(context.Background(), record)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	accepted := 0
	for err := range errs {
		if err == nil {
			accepted++
		} else if !errors.Is(err, persistence.ErrStreamHighWater) {
			t.Fatal(fmt.Errorf("unexpected enqueue error: %w", err))
		}
	}
	if accepted != 8 || fake.length != 8 {
		t.Fatalf("accepted=%d length=%d, want atomic 80%% cap of 8", accepted, fake.length)
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

func TestWaitForCapacityPausesUntilResumeThreshold(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.CommWriteStreamBudgetOverrides = "wecredit:sms:100"
	fake := &fakeProducerRedis{length: 80, xaddID: "1-0"}
	producer := persistence.NewProducer(fake)
	restore := persistence.SetAdmissionVerifyIntervalForTest(10 * time.Millisecond)
	t.Cleanup(restore)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- producer.WaitForCapacity(ctx, "wecredit", "sms") }()
	select {
	case err := <-done:
		t.Fatalf("paused admission returned early: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	fake.mu.Lock()
	fake.length = 60
	fake.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatalf("resume failed: %v", err)
	}
}

func TestWaitForCapacityHonorsCancellation(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.CommWriteStreamBudgetOverrides = "wecredit:sms:100"
	producer := persistence.NewProducer(&fakeProducerRedis{length: 80})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := producer.WaitForCapacity(ctx, "wecredit", "sms"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}

func TestBatchLimitsStayBelowSQLServerParameterLimit(t *testing.T) {
	if got := persistence.BatchLimit(persistence.MapperMarketingSMS); got != 140 {
		t.Fatalf("SMS batch = %d, want 140", got)
	}
	if got := persistence.BatchParameterCount(persistence.MapperMarketingSMS); got != 1820 || got > 2100 {
		t.Fatalf("SMS parameters = %d, want 1820 and <=2100", got)
	}
	if got := persistence.BatchLimit(persistence.MapperMarketingWhatsapp); got != 70 {
		t.Fatalf("WhatsApp batch = %d, want 70", got)
	}
	if got := persistence.BatchParameterCount(persistence.MapperMarketingWhatsapp); got != 1470 || got > 2100 {
		t.Fatalf("WhatsApp parameters = %d, want 1470 and <=2100", got)
	}
}

func TestBusinessLockKeysUseLaneAndNonPIIIdentity(t *testing.T) {
	whatsapp := persistence.BusinessLockKeyForTest("wecredit:whatsapp", persistence.Record{
		Mapper: persistence.MapperMarketingWhatsapp, SourceRowID: 42,
	})
	if whatsapp != "commwrite:lock:{wecredit:whatsapp}:42" {
		t.Fatalf("WhatsApp lock key = %q", whatsapp)
	}
	sms := persistence.BusinessLockKeyForTest("wecredit:sms", persistence.Record{
		Mapper: persistence.MapperMarketingSMS, Source: "secret-source", SourceRowID: 42, Channel: "SMS",
	})
	if !strings.HasPrefix(sms, "commwrite:lock:{wecredit:sms}:") || strings.Contains(sms, "secret-source") || strings.Contains(sms, ":42") {
		t.Fatalf("SMS lock key exposed identity: %q", sms)
	}
}

func TestBusinessLockOwnershipProtectsRenewAndRelease(t *testing.T) {
	client := &fakeBusinessLockRedis{}
	ctx := context.Background()
	if ok, err := persistence.AcquireBusinessLockForTest(ctx, client, "key", "owner-a"); err != nil || !ok {
		t.Fatalf("acquire owner-a ok=%t err=%v", ok, err)
	}
	if ok, err := persistence.AcquireBusinessLockForTest(ctx, client, "key", "owner-b"); err != nil || ok {
		t.Fatalf("duplicate acquire ok=%t err=%v", ok, err)
	}
	if ok, err := persistence.RenewBusinessLockForTest(ctx, client, "key", "owner-b"); err != nil || ok {
		t.Fatalf("wrong-token renew ok=%t err=%v", ok, err)
	}
	if ok, err := persistence.ReleaseBusinessLockForTest(ctx, client, "key", "owner-b"); err != nil || ok {
		t.Fatalf("wrong-token release ok=%t err=%v", ok, err)
	}
	if ok, err := persistence.RenewBusinessLockForTest(ctx, client, "key", "owner-a"); err != nil || !ok {
		t.Fatalf("owner renew ok=%t err=%v", ok, err)
	}
	if ok, err := persistence.ReleaseBusinessLockForTest(ctx, client, "key", "owner-a"); err != nil || !ok {
		t.Fatalf("owner release ok=%t err=%v", ok, err)
	}
	if ok, err := persistence.AcquireBusinessLockForTest(ctx, client, "key", "owner-b"); err != nil || !ok {
		t.Fatalf("reacquire after release ok=%t err=%v", ok, err)
	}
}

func TestBusinessLocksAllowDifferentIdentitiesConcurrently(t *testing.T) {
	client := &fakeBusinessLockRedis{}
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for _, key := range []string{"key-a", "key-b"} {
		wg.Add(1)
		go func(lockKey string) {
			defer wg.Done()
			ok, _ := persistence.AcquireBusinessLockForTest(ctx, client, lockKey, lockKey+"-owner")
			results <- ok
		}(key)
	}
	wg.Wait()
	close(results)
	for ok := range results {
		if !ok {
			t.Fatal("distinct identities should acquire concurrently")
		}
	}
}

func TestAutoClaimContinuesAcrossEmptyCursorPage(t *testing.T) {
	if !persistence.ContinueAutoClaimForTest("0-0", "123-0", false) {
		t.Fatal("XAUTOCLAIM must continue when Redis advances an empty page cursor")
	}
	if persistence.ContinueAutoClaimForTest("123-0", "0-0", false) {
		t.Fatal("XAUTOCLAIM must stop only at the terminal cursor")
	}
	if persistence.ContinueAutoClaimForTest("123-0", "123-0", false) {
		t.Fatal("XAUTOCLAIM must stop a non-advancing cursor")
	}
}

func TestTransientSQLWriteRetriesImmediatelyWithBoundedAttempts(t *testing.T) {
	var calls int
	restore := persistence.SetWriteRecordsAttemptForTest(func(context.Context, []persistence.Record) error {
		calls++
		if calls < 3 {
			return errors.New("deadlock victim")
		}
		return nil
	}, time.Millisecond)
	t.Cleanup(restore)

	err := persistence.WriteRecordsWithRetryForTest(context.Background(), "wecredit:sms", []persistence.Record{{Mapper: persistence.MapperMarketingSMS}})
	if err != nil {
		t.Fatalf("retry did not recover: %v", err)
	}
	if calls != 3 {
		t.Fatalf("write attempts = %d, want 3", calls)
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
