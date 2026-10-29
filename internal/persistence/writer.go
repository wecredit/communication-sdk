package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/metrics"
	commredis "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

const (
	writerGroup          = "commwrite-writers-v1"
	writerBatchSize      = int64(100)
	writerFirstReadBlock = 500 * time.Millisecond
	writerBatchWindow    = 50 * time.Millisecond
	writerClaimIdle      = 60 * time.Second
	writerClaimEvery     = 30 * time.Second
	writerMaxBackoff     = 30 * time.Second
	writerMetricEvery    = time.Minute
	writerLeaseTTL       = 15 * time.Second
	writerLeaseRenew     = 5 * time.Second
	writerLeaseRetry     = 2 * time.Second
)

var renewLeaseScript = redislib.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

var releaseLeaseScript = redislib.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type Runtime struct {
	ctx   context.Context
	redis *redislib.Client
	wg    sync.WaitGroup
	ready map[string]bool
	mu    sync.RWMutex
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
	if err := verifyRedisSafety(ctx, runtime.redis); err != nil {
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
	r.runWriter(ctx, stream, lane, mapper)
	cancel()
	<-renewDone
	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer releaseCancel()
	_, _ = releaseLeaseScript.Run(releaseCtx, r.redis, []string{leaseKey}, token).Result()
}

func (r *Runtime) runWriter(ctx context.Context, stream, lane, mapper string) {
	consumer := writerConsumerName(lane)
	nextClaim := time.Now()
	nextMetric := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		now := time.Now()
		if !now.Before(nextMetric) {
			r.emitStreamMetrics(ctx, stream, lane)
			nextMetric = now.Add(writerMetricEvery)
		}
		if !now.Before(nextClaim) {
			r.reclaimPending(ctx, stream, lane, mapper, consumer)
			nextClaim = now.Add(writerClaimEvery)
			continue
		}

		messages, err := r.readBatch(ctx, stream, consumer)
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
	start := "0-0"
	for {
		messages, next, err := r.redis.XAutoClaim(ctx, &redislib.XAutoClaimArgs{
			Stream: stream, Group: writerGroup, Consumer: consumer,
			MinIdle: writerClaimIdle, Start: start, Count: writerBatchSize,
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
			r.processMessages(ctx, stream, lane, mapper, messages)
		}
		if len(messages) == 0 || next == "0-0" || ctx.Err() != nil {
			return
		}
		start = next
	}
}

func (r *Runtime) readBatch(ctx context.Context, stream, consumer string) ([]redislib.XMessage, error) {
	messages, err := r.readGroup(ctx, stream, consumer, writerBatchSize, writerFirstReadBlock)
	if err != nil || len(messages) == 0 || int64(len(messages)) >= writerBatchSize {
		return messages, err
	}
	deadline := time.Now().Add(writerBatchWindow)
	for int64(len(messages)) < writerBatchSize {
		remaining := time.Until(deadline)
		if remaining < time.Millisecond {
			break
		}
		more, readErr := r.readGroup(ctx, stream, consumer, writerBatchSize-int64(len(messages)), remaining)
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
	records := make([]Record, 0, len(messages))
	validMessages := make([]redislib.XMessage, 0, len(messages))
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
		records = append(records, record)
		validMessages = append(validMessages, message)
	}
	if len(records) == 0 {
		return
	}
	started := time.Now()
	backoff := time.Second
	for {
		err := writeRecords(database.DBMarketing, records)
		if err == nil {
			parts := strings.SplitN(lane, ":", 2)
			metrics.ValueByChannel("CommWriteInsertMilliseconds", "Milliseconds", parts[0], parts[1], float64(time.Since(started).Milliseconds()))
			r.ackAndDelete(stream, validMessages)
			return
		}
		if isDataError(err) && len(records) > 1 {
			r.processIndividually(ctx, stream, lane, records, validMessages)
			return
		}
		if isDataError(err) {
			if moveErr := r.moveToPoison(stream, lane, validMessages[0], err); moveErr != nil {
				utils.Error(fmt.Errorf("commwrite poison move lane=%s: %w", lane, moveErr))
			}
			return
		}
		metrics.CountByReason("CommWriteInsertRetry", lane, retryReason(err), 1)
		utils.Error(fmt.Errorf("commwrite insert retry lane=%s: %w", lane, err))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			if backoff < writerMaxBackoff {
				backoff *= 2
				if backoff > writerMaxBackoff {
					backoff = writerMaxBackoff
				}
			}
		}
	}
}

func (r *Runtime) processIndividually(ctx context.Context, stream, lane string, records []Record, messages []redislib.XMessage) {
	for i, record := range records {
		backoff := time.Second
		for {
			err := writeRecords(database.DBMarketing, []Record{record})
			if err == nil {
				r.ackAndDelete(stream, []redislib.XMessage{messages[i]})
				break
			}

			if isDataError(err) {
				if moveErr := r.moveToPoison(stream, lane, messages[i], err); moveErr != nil {
					utils.Error(fmt.Errorf("commwrite poison move lane=%s: %w", lane, moveErr))
				}
				break
			}
			metrics.CountByReason("CommWriteInsertRetry", lane, retryReason(err), 1)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				if backoff < writerMaxBackoff {
					backoff *= 2
					if backoff > writerMaxBackoff {
						backoff = writerMaxBackoff
					}
				}
			}
		}
	}
}

func (r *Runtime) ackAndDelete(stream string, messages []redislib.XMessage) {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	if len(ids) == 0 {
		return
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
	_, err := r.redis.TxPipelined(ctx, func(pipe redislib.Pipeliner) error {
		pipe.XAdd(ctx, &redislib.XAddArgs{Stream: PoisonStreamKey(lane), Values: values})
		pipe.XAck(ctx, stream, writerGroup, message.ID)
		pipe.XDel(ctx, stream, message.ID)
		return nil
	})
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

func writerConsumerName(lane string) string {
	host, _ := os.Hostname()
	return strings.NewReplacer(":", "-", ".", "-").Replace(lane + "-" + host)
}

func leaseKey(lane string) string { return streamPrefix(lane) + ":lease" }

func PoisonStreamKey(lane string) string { return streamPrefix(lane) + ":poison" }

func anyAsyncConfigured() bool {
	for key := range supportedChannels {
		parts := strings.SplitN(key, ":", 2)
		if AsyncEnabled(parts[0], parts[1]) {
			return true
		}
	}
	return false
}

func verifyRedisSafety(ctx context.Context, client *redislib.Client) error {
	info, err := client.Info(ctx, "server").Result()
	if err != nil {
		return fmt.Errorf("verify Redis version: %w", err)
	}
	version := ""
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			version = strings.TrimSpace(strings.TrimPrefix(line, "redis_version:"))
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
	policy, err := client.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return fmt.Errorf("verify Redis maxmemory-policy: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(policy["maxmemory-policy"]), "noeviction") {
		return fmt.Errorf("Redis maxmemory-policy=%q; async persistence requires noeviction", policy["maxmemory-policy"])
	}
	return nil
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
