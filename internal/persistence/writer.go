package persistence

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/metrics"
	commredis "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

const (
	writerGroup             = "commwrite-writers-v1"
	writerFirstReadBlock    = 500 * time.Millisecond
	writerBatchWindow       = 50 * time.Millisecond
	writerClaimIdle         = 60 * time.Second
	writerClaimEvery        = 30 * time.Second
	writerMetricEvery       = time.Minute
	writerLeaseTTL          = 15 * time.Second
	writerLeaseRenew        = 5 * time.Second
	writerLeaseRetry        = 2 * time.Second
	writerBusinessLockTTL   = 120 * time.Second
	writerBusinessLockRenew = 40 * time.Second
	writerSQLMaxAttempts    = 3
)

var writerSQLRetryBase = time.Second
var writeRecordsAttempt = func(ctx context.Context, records []Record) error {
	return writeRecords(ctx, database.DBMarketing, records)
}

func SetWriteRecordsAttemptForTest(attempt func(context.Context, []Record) error, retryBase time.Duration) func() {
	previousAttempt := writeRecordsAttempt
	previousRetryBase := writerSQLRetryBase
	writeRecordsAttempt = attempt
	writerSQLRetryBase = retryBase
	return func() {
		writeRecordsAttempt = previousAttempt
		writerSQLRetryBase = previousRetryBase
	}
}

const renewOwnedLockLua = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`

const releaseOwnedLockLua = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`

var renewLeaseScript = redislib.NewScript(renewOwnedLockLua)
var releaseLeaseScript = redislib.NewScript(releaseOwnedLockLua)

var moveToPoisonScript = redislib.NewScript(`
redis.call("XADD", KEYS[2], "*", unpack(ARGV, 3))
redis.call("XACK", KEYS[1], ARGV[1], ARGV[2])
return redis.call("XDEL", KEYS[1], ARGV[2])
`)

type Runtime struct {
	ctx   context.Context
	redis *redislib.Client
	wg    sync.WaitGroup
	ready map[string]bool
	mu    sync.RWMutex
}

type BusinessLockRedis interface {
	SetNX(context.Context, string, interface{}, time.Duration) *redislib.BoolCmd
	Eval(context.Context, string, []string, ...interface{}) *redislib.Cmd
}

func acquireBusinessLock(ctx context.Context, client BusinessLockRedis, key, token string) (bool, error) {
	return client.SetNX(ctx, key, token, writerBusinessLockTTL).Result()
}

func renewBusinessLock(ctx context.Context, client BusinessLockRedis, key, token string) (bool, error) {
	result, err := client.Eval(ctx, renewOwnedLockLua, []string{key}, token, writerBusinessLockTTL.Milliseconds()).Int()
	return result == 1, err
}

func releaseBusinessLock(ctx context.Context, client BusinessLockRedis, key, token string) (bool, error) {
	result, err := client.Eval(ctx, releaseOwnedLockLua, []string{key}, token).Int()
	return result == 1, err
}

func AcquireBusinessLockForTest(ctx context.Context, client BusinessLockRedis, key, token string) (bool, error) {
	return acquireBusinessLock(ctx, client, key, token)
}

func RenewBusinessLockForTest(ctx context.Context, client BusinessLockRedis, key, token string) (bool, error) {
	return renewBusinessLock(ctx, client, key, token)
}

func ReleaseBusinessLockForTest(ctx context.Context, client BusinessLockRedis, key, token string) (bool, error) {
	return releaseBusinessLock(ctx, client, key, token)
}

var defaultRuntime = &Runtime{ready: make(map[string]bool)}

// Start prepares one Redis-backed write lane per enabled client/channel. Every
// comm-sdk task runs a lease contender, but only one task may hold a lane lease
// at a time. ECS scale-out therefore does not multiply SQL write concurrency.
func Start(ctx context.Context) error {
	ConfigureProducer()
	runtime := &Runtime{ctx: ctx, redis: commredis.RDB, ready: make(map[string]bool)}
	defaultRuntime = runtime
	if err := validateRequestedAsyncConfiguration(); err != nil {
		return err
	}
	if !anyAsyncConfigured() {
		return nil
	}
	if runtime.redis == nil {
		return fmt.Errorf("async persistence configured but Redis is unavailable")
	}
	if database.DBMarketing == nil {
		return fmt.Errorf("async persistence configured but Marketing database is unavailable")
	}
	if err := VerifyRedisSafety(ctx, runtime.redis, config.Configs.RedisAddress); err != nil {
		return err
	}

	type laneConfig struct {
		key, stream, mapper string
	}
	lanes := make([]laneConfig, 0, len(supportedChannels))
	for key, mapper := range supportedChannels {
		parts := strings.SplitN(key, ":", 2)
		if !AsyncEnabled(parts[0], parts[1]) {
			continue
		}
		stream := StreamKey(parts[0], parts[1])
		if err := runtime.ensureGroup(ctx, stream); err != nil {
			return err
		}
		lanes = append(lanes, laneConfig{key: key, stream: stream, mapper: mapper})
	}

	for _, lane := range lanes {
		runtime.mu.Lock()
		runtime.ready[lane.key] = true
		runtime.mu.Unlock()
		runtime.wg.Add(1)
		go runtime.runLeaseContender(lane.stream, lane.key, lane.mapper)
	}
	return nil
}

func validateRequestedAsyncConfiguration() error {
	if err := config.ValidateCommWriteWorkerOverrides(config.Configs.CommWriteWorkerOverrides); err != nil {
		return err
	}
	for _, entry := range strings.Split(config.Configs.CommWriteWorkerOverrides, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) == 3 && !AsyncEnabled(parts[0], parts[1]) {
			return fmt.Errorf("commwrite worker override requires enabled async lane %s:%s", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
	requested := requestedAsyncChannels()
	if len(requested) == 0 {
		return nil
	}
	for _, key := range requested {
		if _, supported := supportedChannels[key]; !supported {
			return fmt.Errorf("unsupported async persistence channel %q", key)
		}
		parts := strings.SplitN(key, ":", 2)
		if streamBudget(parts[0], parts[1]) < 1 {
			return fmt.Errorf("positive stream budget is required for %s", key)
		}
	}
	return nil
}

func Wait() { defaultRuntime.wg.Wait() }

// WriterActive reports whether this process successfully prepared the shared
// lane. It intentionally does not mean this task owns the global writer lease;
// producer-only tasks must still enqueue while another task owns that lease.
func WriterActive(client, channel string) bool {
	defaultRuntime.mu.RLock()
	defer defaultRuntime.mu.RUnlock()
	return defaultRuntime.ready[channelKey(client, channel)]
}

func SetWriterActiveForTest(client, channel string, active bool) func() {
	key := channelKey(client, channel)
	defaultRuntime.mu.Lock()
	previous, existed := defaultRuntime.ready[key]
	defaultRuntime.ready[key] = active
	defaultRuntime.mu.Unlock()
	return func() {
		defaultRuntime.mu.Lock()
		defer defaultRuntime.mu.Unlock()
		if existed {
			defaultRuntime.ready[key] = previous
		} else {
			delete(defaultRuntime.ready, key)
		}
	}
}

func (r *Runtime) ensureGroup(ctx context.Context, stream string) error {
	err := r.redis.XGroupCreateMkStream(ctx, stream, writerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group for %s: %w", stream, err)
	}
	return nil
}

func (r *Runtime) runLeaseContender(stream, lane, mapper string) {
	defer r.wg.Done()
	host, _ := os.Hostname()
	token := fmt.Sprintf("%s-%d-%d", host, os.Getpid(), time.Now().UnixNano())
	leaseKey := leaseKey(lane)
	ticker := time.NewTicker(writerLeaseRetry)
	defer ticker.Stop()

	for {
		acquired, err := r.redis.SetNX(r.ctx, leaseKey, token, writerLeaseTTL).Result()
		if err != nil && !errors.Is(err, context.Canceled) {
			utils.Error(fmt.Errorf("commwrite lease acquire lane=%s: %w", lane, err))
		}
		if acquired {
			r.runLeasedWriter(stream, lane, mapper, leaseKey, token)
		}
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runtime) runLeasedWriter(stream, lane, mapper, leaseKey, token string) {
	ctx, cancel := context.WithCancel(r.ctx)
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(writerLeaseRenew)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				result, err := renewLeaseScript.Run(ctx, r.redis, []string{leaseKey}, token, writerLeaseTTL.Milliseconds()).Int()
				if err != nil || result != 1 {
					if err != nil && !errors.Is(err, context.Canceled) {
						utils.Error(fmt.Errorf("commwrite lease renew lane=%s: %w", lane, err))
					}
					cancel()
					return
				}
			}
		}
	}()

	metrics.CountByReason("CommWriteLeaseAcquired", lane, "acquired", 1)
	parts := strings.SplitN(lane, ":", 2)
	workers, err := config.CommWriteWorkerCount(config.Configs.CommWriteWorkerOverrides, parts[0], parts[1])
	if err != nil {
		utils.Error(fmt.Errorf("commwrite worker configuration lane=%s: %w", lane, err))
		workers = 1
	}
	instance := uuid.NewString()
	var workersWG sync.WaitGroup
	metrics.GaugeByChannel("CommWriteActiveWorkers", "Count", parts[0], parts[1], float64(workers))
	for index := 0; index < workers; index++ {
		workersWG.Add(1)
		go func(workerIndex int) {
			defer workersWG.Done()
			consumer := writerConsumerName(lane, instance, workerIndex)
			r.runWriter(ctx, stream, lane, mapper, consumer, workerIndex == 0)
		}(index)
	}
	workersWG.Wait()
	cancel()
	<-renewDone
	metrics.GaugeByChannel("CommWriteActiveWorkers", "Count", parts[0], parts[1], 0)
	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer releaseCancel()
	_, _ = releaseLeaseScript.Run(releaseCtx, r.redis, []string{leaseKey}, token).Result()
}

func (r *Runtime) runWriter(ctx context.Context, stream, lane, mapper, consumer string, coordinator bool) {
	nextClaim := time.Now()
	nextMetric := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		now := time.Now()
		if coordinator && !now.Before(nextMetric) {
			r.emitStreamMetrics(ctx, stream, lane)
			nextMetric = now.Add(writerMetricEvery)
		}
		if coordinator && !now.Before(nextClaim) {
			r.reclaimPending(ctx, stream, lane, mapper, consumer)
			nextClaim = now.Add(writerClaimEvery)
			continue
		}

		messages, err := r.readBatch(ctx, stream, consumer, mapper)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if isNoGroup(err) {
			_ = r.ensureGroup(ctx, stream)
			continue
		}
		if err != nil && err != redislib.Nil {
			utils.Error(fmt.Errorf("commwrite XREADGROUP lane=%s: %w", lane, err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		r.processMessages(ctx, stream, lane, mapper, messages)
	}
}

func (r *Runtime) reclaimPending(ctx context.Context, stream, lane, mapper, consumer string) {
	batchSize := BatchLimit(mapper)
	start := "0-0"
	for {
		messages, next, err := r.redis.XAutoClaim(ctx, &redislib.XAutoClaimArgs{
			Stream: stream, Group: writerGroup, Consumer: consumer,
			MinIdle: writerClaimIdle, Start: start, Count: batchSize,
		}).Result()
		if isNoGroup(err) {
			_ = r.ensureGroup(ctx, stream)
			return
		}
		if err != nil && err != redislib.Nil && !errors.Is(err, context.Canceled) {
			utils.Error(fmt.Errorf("commwrite XAUTOCLAIM lane=%s: %w", lane, err))
			return
		}
		if len(messages) > 0 {
			metrics.CountByReason("CommWriteReclaimed", lane, "pending", len(messages))
			r.processMessages(ctx, stream, lane, mapper, messages)
		}
		if !continueAutoClaim(start, next, ctx.Err() != nil) {
			if next == start && next != "0-0" {
				metrics.CountByReason("CommWriteReclaimCursorStalled", lane, "same_cursor", 1)
			}
			return
		}
		// Redis may return an empty page while advancing past deleted PEL
		// entries. Keep scanning the returned cursor so older live pending
		// entries cannot remain stranded behind those holes.
		start = next
	}
}

func continueAutoClaim(start, next string, canceled bool) bool {
	return !canceled && next != "0-0" && next != start
}

func ContinueAutoClaimForTest(start, next string, canceled bool) bool {
	return continueAutoClaim(start, next, canceled)
}

func (r *Runtime) readBatch(ctx context.Context, stream, consumer, mapper string) ([]redislib.XMessage, error) {
	batchSize := BatchLimit(mapper)
	messages, err := r.readGroup(ctx, stream, consumer, batchSize, writerFirstReadBlock)
	if err != nil || len(messages) == 0 || int64(len(messages)) >= batchSize {
		return messages, err
	}
	deadline := time.Now().Add(writerBatchWindow)
	for int64(len(messages)) < batchSize {
		remaining := time.Until(deadline)
		if remaining < time.Millisecond {
			break
		}
		more, readErr := r.readGroup(ctx, stream, consumer, batchSize-int64(len(messages)), remaining)
		if readErr == redislib.Nil {
			break
		}
		if readErr != nil {
			return messages, readErr
		}
		messages = append(messages, more...)
	}
	return messages, nil
}

func (r *Runtime) readGroup(ctx context.Context, stream, consumer string, count int64, block time.Duration) ([]redislib.XMessage, error) {
	streams, err := r.redis.XReadGroup(ctx, &redislib.XReadGroupArgs{
		Group: writerGroup, Consumer: consumer, Streams: []string{stream, ">"},
		Count: count, Block: block,
	}).Result()
	if err != nil {
		return nil, err
	}
	var messages []redislib.XMessage
	for _, result := range streams {
		messages = append(messages, result.Messages...)
	}
	return messages, nil
}

func (r *Runtime) processMessages(ctx context.Context, stream, lane, mapper string, messages []redislib.XMessage) {
	if len(messages) == 0 {
		return
	}
	groupsByKey := make(map[string]*recordGroup, len(messages))
	groups := make([]*recordGroup, 0, len(messages))
	for _, message := range messages {
		record, err := recordFromMessage(message)
		if err != nil || record.Mapper != mapper {
			if err == nil {
				err = fmt.Errorf("mapper %q does not belong to lane %s", record.Mapper, lane)
			}
			if moveErr := r.moveToPoison(stream, lane, message, err); moveErr != nil {
				utils.Error(fmt.Errorf("commwrite poison move lane=%s: %w", lane, moveErr))
			}
			continue
		}
		identity := businessIdentity(record)
		group := groupsByKey[identity]
		if group == nil {
			group = &recordGroup{record: record, identity: identity}
			groupsByKey[identity] = group
			groups = append(groups, group)
		}
		group.messages = append(group.messages, message)
	}
	if len(groups) == 0 {
		return
	}

	acquired := make([]*recordGroup, 0, len(groups))
	for _, group := range groups {
		group.lockKey = businessLockKey(lane, group.record)
		group.token = uuid.NewString()
	}
	commands := make([]*redislib.BoolCmd, len(groups))
	r.redis.Pipelined(ctx, func(pipe redislib.Pipeliner) error {
		for index, group := range groups {
			commands[index] = pipe.SetNX(ctx, group.lockKey, group.token, writerBusinessLockTTL)
		}
		return nil
	})
	for index, group := range groups {
		ok, err := commands[index].Result()
		if err != nil {
			metrics.CountByReason("CommWriteLockContention", lane, "redis_error", 1)
			continue
		}
		if !ok {
			metrics.CountByReason("CommWriteLockContention", lane, "owned", 1)
			continue
		}
		acquired = append(acquired, group)
	}
	if len(acquired) == 0 {
		return
	}
	defer r.releaseBusinessLocks(lane, acquired)
	r.processLockedGroups(ctx, stream, lane, acquired)
}

type recordGroup struct {
	record   Record
	identity string
	messages []redislib.XMessage
	lockKey  string
	token    string
}

func (r *Runtime) processLockedGroups(parent context.Context, stream, lane string, groups []*recordGroup) {
	ctx, cancel := context.WithCancel(parent)
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(writerBusinessLockRenew)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !r.renewBusinessLocks(ctx, lane, groups) {
					cancel()
					return
				}
			}
		}
	}()

	records := make([]Record, 0, len(groups))
	for _, group := range groups {
		records = append(records, group.record)
	}
	started := time.Now()
	err := writeRecordsWithRetry(ctx, lane, records)
	if err == nil {
		if r.renewBusinessLocks(ctx, lane, groups) {
			allMessages := messagesForGroups(groups)
			if ackErr := r.ackAndDelete(stream, allMessages); ackErr == nil {
				metrics.CountByReason("CommWriteBatchSuccess", lane, "committed", 1)
				parts := strings.SplitN(lane, ":", 2)
				metrics.ValueByChannel("CommWriteInsertMilliseconds", "Milliseconds", parts[0], parts[1], float64(time.Since(started).Milliseconds()))
			}
		}
		cancel()
		<-renewDone
		return
	}

	if isDataError(err) && len(groups) > 1 && ctx.Err() == nil {
		for _, group := range groups {
			r.processLockedGroup(ctx, stream, lane, group)
		}
	} else if isDataError(err) && ctx.Err() == nil {
		r.poisonLockedGroup(ctx, stream, lane, groups[0], err)
	}
	cancel()
	<-renewDone
}

func (r *Runtime) processLockedGroup(ctx context.Context, stream, lane string, group *recordGroup) {
	err := writeRecordsWithRetry(ctx, lane, []Record{group.record})
	if err == nil {
		if r.renewBusinessLocks(ctx, lane, []*recordGroup{group}) {
			if ackErr := r.ackAndDelete(stream, group.messages); ackErr == nil {
				metrics.CountByReason("CommWriteBatchSuccess", lane, "isolated", 1)
			}
		}
		return
	}
	if isDataError(err) && ctx.Err() == nil {
		r.poisonLockedGroup(ctx, stream, lane, group, err)
		return
	}
}

func writeRecordsWithRetry(ctx context.Context, lane string, records []Record) error {
	backoff := writerSQLRetryBase
	var err error
	for attempt := 1; attempt <= writerSQLMaxAttempts; attempt++ {
		err = writeRecordsAttempt(ctx, records)
		if err == nil || isDataError(err) || ctx.Err() != nil {
			return err
		}
		metrics.CountByReason("CommWriteTransientRetry", lane, retryReason(err), 1)
		if attempt == writerSQLMaxAttempts {
			return err
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
	}
	return err
}

func WriteRecordsWithRetryForTest(ctx context.Context, lane string, records []Record) error {
	return writeRecordsWithRetry(ctx, lane, records)
}

func (r *Runtime) poisonLockedGroup(ctx context.Context, stream, lane string, group *recordGroup, cause error) {
	if !r.renewBusinessLocks(ctx, lane, []*recordGroup{group}) {
		return
	}
	for _, message := range group.messages {
		if moveErr := r.moveToPoison(stream, lane, message, cause); moveErr != nil {
			utils.Error(fmt.Errorf("commwrite poison move lane=%s: %w", lane, moveErr))
			return
		}
	}
}

func (r *Runtime) ackAndDelete(stream string, messages []redislib.XMessage) error {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.redis.TxPipelined(ctx, func(pipe redislib.Pipeliner) error {
		pipe.XAck(ctx, stream, writerGroup, ids...)
		pipe.XDel(ctx, stream, ids...)
		return nil
	})
	if err != nil {
		utils.Error(fmt.Errorf("commwrite XACK/XDEL stream=%s: %w", stream, err))
	}
	return err
}

func businessIdentity(record Record) string {
	if record.Mapper == MapperMarketingWhatsapp {
		return strconv.FormatInt(record.SourceRowID, 10)
	}
	canonical := strings.ToLower(strings.TrimSpace(record.Source)) + "\x00" +
		strconv.FormatInt(record.SourceRowID, 10) + "\x00" + strings.ToUpper(strings.TrimSpace(record.Channel))
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%x", sum[:])
}

func businessLockKey(lane string, record Record) string {
	return "commwrite:lock:{" + lane + "}:" + businessIdentity(record)
}

func BusinessLockKeyForTest(lane string, record Record) string {
	return businessLockKey(lane, record)
}

func messagesForGroups(groups []*recordGroup) []redislib.XMessage {
	count := 0
	for _, group := range groups {
		count += len(group.messages)
	}
	messages := make([]redislib.XMessage, 0, count)
	for _, group := range groups {
		messages = append(messages, group.messages...)
	}
	return messages
}

func (r *Runtime) renewBusinessLocks(ctx context.Context, lane string, groups []*recordGroup) bool {
	commands := make([]*redislib.Cmd, len(groups))
	r.redis.Pipelined(ctx, func(pipe redislib.Pipeliner) error {
		for index, group := range groups {
			commands[index] = pipe.Eval(ctx, renewOwnedLockLua, []string{group.lockKey}, group.token, writerBusinessLockTTL.Milliseconds())
		}
		return nil
	})
	for index := range groups {
		result, err := commands[index].Int()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				metrics.CountByReason("CommWriteLockRenewalFailure", lane, "redis", 1)
			}
			return false
		}
		if result != 1 {
			metrics.CountByReason("CommWriteLockRenewalFailure", lane, "ownership_lost", 1)
			return false
		}
	}
	return true
}

func (r *Runtime) releaseBusinessLocks(lane string, groups []*recordGroup) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	commands := make([]*redislib.Cmd, len(groups))
	r.redis.Pipelined(ctx, func(pipe redislib.Pipeliner) error {
		for index, group := range groups {
			commands[index] = pipe.Eval(ctx, releaseOwnedLockLua, []string{group.lockKey}, group.token)
		}
		return nil
	})
	for index := range groups {
		result, err := commands[index].Int()
		if err != nil {
			metrics.CountByReason("CommWriteLockReleaseFailure", lane, "redis", 1)
			continue
		}
		if result != 1 {
			metrics.CountByReason("CommWriteLockReleaseFailure", lane, "ownership_lost", 1)
		}
	}
}

func (r *Runtime) moveToPoison(stream, lane string, message redislib.XMessage, cause error) error {
	values := make(map[string]interface{}, len(message.Values)+3)
	for key, value := range message.Values {
		values[key] = value
	}
	values["source_stream_id"] = message.ID
	values["poison_reason"] = strings.Join(strings.Fields(cause.Error()), " ")
	values["poisoned_at"] = time.Now().UTC().Format(time.RFC3339Nano)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := make([]interface{}, 2, 2+len(values)*2)
	args[0] = writerGroup
	args[1] = message.ID
	for key, value := range values {
		args = append(args, key, value)
	}
	_, err := moveToPoisonScript.Run(ctx, r.redis, []string{stream, PoisonStreamKey(lane)}, args...).Result()
	if err != nil {
		metrics.CountByReason("CommWritePoisonPreservationFailure", lane, "redis", 1)
		return err
	}
	metrics.CountByReason("CommWritePoison", lane, "preserved", 1)
	return nil
}

func (r *Runtime) emitStreamMetrics(ctx context.Context, stream, lane string) {
	parts := strings.SplitN(lane, ":", 2)
	length, lengthErr := r.redis.XLen(ctx, stream).Result()
	poisonLength, poisonErr := r.redis.XLen(ctx, PoisonStreamKey(lane)).Result()
	pending, pendingErr := r.redis.XPending(ctx, stream, writerGroup).Result()
	oldestAge := float64(0)
	entries, rangeErr := r.redis.XRangeN(ctx, stream, "-", "+", 1).Result()
	if rangeErr == nil && len(entries) > 0 {
		if milliseconds, _, ok := strings.Cut(entries[0].ID, "-"); ok {
			if createdAt, err := strconv.ParseInt(milliseconds, 10, 64); err == nil {
				oldestAge = time.Since(time.UnixMilli(createdAt)).Seconds()
				if oldestAge < 0 {
					oldestAge = 0
				}
			}
		}
	}
	if isNoGroup(pendingErr) {
		_ = r.ensureGroup(ctx, stream)
		return
	}
	if lengthErr != nil || poisonErr != nil || pendingErr != nil || (rangeErr != nil && rangeErr != redislib.Nil) {
		utils.Error(fmt.Errorf("commwrite metric read failed lane=%s length=%v poison=%v pending=%v range=%v", lane, lengthErr, poisonErr, pendingErr, rangeErr))
		return
	}
	metrics.GaugeByChannel("CommWriteStreamLength", "Count", parts[0], parts[1], float64(length))
	metrics.GaugeByChannel("CommWritePendingCount", "Count", parts[0], parts[1], float64(pending.Count))
	metrics.GaugeByChannel("CommWritePoisonCount", "Count", parts[0], parts[1], float64(poisonLength))
	metrics.GaugeByChannel("CommWriteOldestEntryAgeSeconds", "Seconds", parts[0], parts[1], oldestAge)
	metrics.GaugeByChannel("CommWriteWriterPresent", "Count", parts[0], parts[1], 1)
}

func writerConsumerName(lane, instance string, worker int) string {
	host, _ := os.Hostname()
	name := fmt.Sprintf("%s-%s-%d-%s-%d", lane, host, os.Getpid(), instance, worker)
	return strings.NewReplacer(":", "-", ".", "-").Replace(name)
}

func leaseKey(lane string) string { return streamPrefix(lane) + ":lease" }

func PoisonStreamKey(lane string) string { return streamPrefix(lane) + ":poison" }

// PoisonEntry is a read-only operator view. Values contains the complete
// original stream record plus source_stream_id, poison_reason and poisoned_at.
type PoisonEntry struct {
	ID     string
	Values map[string]interface{}
}

func PoisonCount(ctx context.Context, client, channel string) (int64, error) {
	if defaultRuntime.redis == nil {
		return 0, fmt.Errorf("commwrite Redis is unavailable")
	}
	return defaultRuntime.redis.XLen(ctx, PoisonStreamKey(channelKey(client, channel))).Result()
}

// InspectPoison returns at most 100 entries and never mutates or replays them.
func InspectPoison(ctx context.Context, client, channel, start string, count int64) ([]PoisonEntry, error) {
	if defaultRuntime.redis == nil {
		return nil, fmt.Errorf("commwrite Redis is unavailable")
	}
	if strings.TrimSpace(start) == "" {
		start = "-"
	}
	if count < 1 || count > 100 {
		count = 100
	}
	entries, err := defaultRuntime.redis.XRangeN(ctx, PoisonStreamKey(channelKey(client, channel)), start, "+", count).Result()
	if err != nil && err != redislib.Nil {
		return nil, err
	}
	result := make([]PoisonEntry, 0, len(entries))
	for _, entry := range entries {
		values := make(map[string]interface{}, len(entry.Values))
		for key, value := range entry.Values {
			values[key] = value
		}
		result = append(result, PoisonEntry{ID: entry.ID, Values: values})
	}
	return result, nil
}

func anyAsyncConfigured() bool {
	for key := range supportedChannels {
		parts := strings.SplitN(key, ":", 2)
		if AsyncEnabled(parts[0], parts[1]) {
			return true
		}
	}
	return false
}

// RedisSafetyClient is the small Redis command surface needed by the startup
// safety check.
type RedisSafetyClient interface {
	Info(ctx context.Context, section ...string) *redislib.StringCmd
	ConfigGet(ctx context.Context, parameter string) *redislib.MapStringStringCmd
}

// VerifyRedisSafety validates the stream recovery command version and eviction
// behavior before async persistence starts. ElastiCache Serverless supports
// XAUTOCLAIM but intentionally restricts CONFIG and fixes maxmemory-policy to
// volatile-lru. The commwrite streams have no TTL and are additionally bounded
// by the configured stream budgets, so only that AWS-managed endpoint skips the
// node-based noeviction check.
func VerifyRedisSafety(ctx context.Context, client RedisSafetyClient, address string) error {
	info, err := client.Info(ctx, "server").Result()
	if err != nil {
		return fmt.Errorf("verify Redis version: %w", err)
	}
	version := ""
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "redis_version:") || strings.HasPrefix(line, "valkey_version:") {
			version = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			break
		}
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return fmt.Errorf("cannot determine Redis version for XAUTOCLAIM")
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 6 || (major == 6 && minor < 2) {
		return fmt.Errorf("Redis %s does not support XAUTOCLAIM; require >=6.2", version)
	}
	if isElastiCacheServerlessAddress(address) {
		utils.Warn("commwrite Redis safety: ElastiCache Serverless uses AWS-managed volatile-lru; CONFIG GET is unavailable; relying on non-expiring streams and configured stream budgets")
		return nil
	}
	policy, err := client.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return fmt.Errorf("verify Redis maxmemory-policy: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(policy["maxmemory-policy"]), "noeviction") {
		return fmt.Errorf("Redis maxmemory-policy=%q; async persistence requires noeviction", policy["maxmemory-policy"])
	}
	return nil
}

func isElastiCacheServerlessAddress(address string) bool {
	host := strings.ToLower(strings.TrimSpace(address))
	host = strings.TrimPrefix(host, "rediss://")
	host = strings.TrimPrefix(host, "redis://")
	if separator := strings.LastIndex(host, ":"); separator >= 0 {
		host = host[:separator]
	}
	return strings.Contains(host, ".serverless.") &&
		(strings.HasSuffix(host, ".cache.amazonaws.com") || strings.HasSuffix(host, ".cache.amazonaws.com.cn"))
}

func retryReason(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "deadlock"):
		return "deadlock"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline"):
		return "timeout"
	case strings.Contains(message, "connect") || strings.Contains(message, "network"):
		return "connectivity"
	default:
		return "other"
	}
}

func isDataError(err error) bool {
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"string or binary data would be truncated", "conversion failed", "error converting data type",
		"cannot insert the value null", "invalid persistence record",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func isNoGroup(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "NOGROUP")
}
