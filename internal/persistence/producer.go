package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/metrics"
	commredis "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

var ErrStreamHighWater = errors.New("commwrite stream is above its pause threshold")

type Producer struct {
	redis  producerRedis
	mu     sync.Mutex
	states map[string]*pressureState
}

type pressureState struct {
	paused   bool
	alerted  bool
	resume   chan struct{}
	pausedAt time.Time
}

type producerRedis interface {
	XLen(context.Context, string) *redis.IntCmd
	Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
}

const cappedXAddScript = `
if redis.call("XLEN", KEYS[1]) >= tonumber(ARGV[1]) then
  return false
end
return redis.call("XADD", KEYS[1], "*", unpack(ARGV, 2))
`

var admissionVerifyEvery = 5 * time.Second

// SetAdmissionVerifyIntervalForTest shortens only the local cross-task
// verification fallback. Production callers must not use it.
func SetAdmissionVerifyIntervalForTest(interval time.Duration) func() {
	previous := admissionVerifyEvery
	admissionVerifyEvery = interval
	return func() { admissionVerifyEvery = previous }
}

func NewProducer(client producerRedis) *Producer {
	return &Producer{redis: client, states: make(map[string]*pressureState)}
}

var defaultProducer = NewProducer(nil)

func SetProducerForTest(producer *Producer) func() {
	previous := defaultProducer
	defaultProducer = producer
	return func() { defaultProducer = previous }
}

func ConfigureProducer() {
	defaultProducer = NewProducer(commredis.RDB)
}

func AsyncEnabled(client, channel string) bool {
	key := channelKey(client, channel)
	if _, supported := supportedChannels[key]; !supported {
		return false
	}

	if streamBudget(client, channel) < 1 {
		return false
	}
	for _, entry := range strings.Split(config.Configs.CommAsyncWriteChannels, ",") {
		if channelKeyFromEntry(entry) == key {
			return true
		}
	}

	return false
}

func requestedAsyncChannels() []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, entry := range strings.Split(config.Configs.CommAsyncWriteChannels, ",") {
		key := channelKeyFromEntry(entry)
		if key == "" {
			continue
		}

		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, key)
		}
	}

	return result
}

func Enqueue(ctx context.Context, record Record) (string, error) {
	return defaultProducer.Enqueue(ctx, record)
}

// CanAccept applies the stream high-water policy before a provider call so an
// affected source lane backpressures without creating new terminal attempts.
func CanAccept(ctx context.Context, client, channel string) error {
	return defaultProducer.CanAccept(ctx, client, channel)
}

// WaitForCapacity blocks without busy-polling while a shared persistence lane
// is above its high-water mark. Redis remains the cross-task source of truth;
// the local notification only makes same-process resume immediate.
func WaitForCapacity(ctx context.Context, client, channel string) error {
	return defaultProducer.WaitForCapacity(ctx, client, channel)
}

func (p *Producer) WaitForCapacity(ctx context.Context, client, channel string) error {
	lane := channelKey(client, channel)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.CanAccept(ctx, client, channel)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		metrics.CountByReason("CommWriteAdmissionDenied", lane, admissionReason(err), 1)
		var resume <-chan struct{}
		if errors.Is(err, ErrStreamHighWater) {
			resume = p.resumeSignal(StreamKey(client, channel))
		}
		timer := time.NewTimer(admissionVerifyEvery)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-resume:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

// EnqueueDurably retries only the persistence append. The caller must seal the
// provider terminal before entering this method, so no retry can re-hit a
// provider. It returns only after XADD succeeds or the caller shuts down.
func EnqueueDurably(ctx context.Context, record Record) (string, error) {
	backoff := time.Second
	for {
		id, err := Enqueue(ctx, record)
		if err == nil {
			return id, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		metrics.CountByReason("CommWriteDurableEnqueueWait", channelKey(record.Client, record.Channel), admissionReason(err), 1)
		if errors.Is(err, ErrStreamHighWater) {
			if waitErr := WaitForCapacity(ctx, record.Client, record.Channel); waitErr != nil {
				return "", waitErr
			}
			continue
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func (p *Producer) CanAccept(ctx context.Context, client, channel string) error {
	if p == nil || p.redis == nil {
		return fmt.Errorf("commwrite Redis is unavailable")
	}

	budget := streamBudget(client, channel)
	if budget < 1 {
		return fmt.Errorf("commwrite stream budget is not configured for %s", channelKey(client, channel))
	}

	stream := StreamKey(client, channel)
	length, err := p.redis.XLen(ctx, stream).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("read commwrite stream length: %w", err)
	}
	return p.applyHighWater(client, channel, stream, length, budget)
}

func (p *Producer) Enqueue(ctx context.Context, record Record) (string, error) {
	if p == nil || p.redis == nil {
		metrics.CountByReason("CommWriteXAddFailure", channelKey(record.Client, record.Channel), "redis_unavailable", 1)
		return "", fmt.Errorf("commwrite Redis is unavailable")
	}

	budget := streamBudget(record.Client, record.Channel)
	if budget < 1 {
		return "", fmt.Errorf("commwrite stream budget is not configured for %s", channelKey(record.Client, record.Channel))
	}

	stream := StreamKey(record.Client, record.Channel)
	length, err := p.redis.XLen(ctx, stream).Result()
	if err != nil && err != redis.Nil {
		metrics.CountByReason("CommWriteXAddFailure", channelKey(record.Client, record.Channel), "xlen", 1)
		return "", fmt.Errorf("read commwrite stream length: %w", err)
	}

	if err := p.applyHighWater(record.Client, record.Channel, stream, length, budget); err != nil {
		return "", err
	}

	limit := (budget*80 + 99) / 100
	values := record.streamValues()
	args := make([]interface{}, 1, 1+len(values)*2)
	args[0] = limit
	for key, value := range values {
		args = append(args, key, value)
	}

	id, err := p.redis.Eval(ctx, cappedXAddScript, []string{stream}, args...).Text()
	if err == redis.Nil {
		_ = p.applyHighWater(record.Client, record.Channel, stream, limit, budget)
		return "", ErrStreamHighWater
	}
	if err != nil {
		metrics.CountByReason("CommWriteXAddFailure", channelKey(record.Client, record.Channel), "xadd", 1)
		return "", fmt.Errorf("XADD %s: %w", stream, err)
	}

	return id, nil
}

func (p *Producer) applyHighWater(client, channel, stream string, length, budget int64) error {
	percent := int(length * 100 / budget)
	lane := channelKey(client, channel)
	p.mu.Lock()
	state := p.states[stream]
	if state == nil {
		state = &pressureState{resume: make(chan struct{})}
		p.states[stream] = state
	}
	if percent < 50 {
		state.alerted = false
	}
	if percent >= 50 && !state.alerted {
		state.alerted = true
		metrics.CountByReason("CommWriteStreamHighWater", lane, "alert_50", 1)
	}
	if percent >= 80 && !state.paused {
		state.paused = true
		state.pausedAt = time.Now()
		state.resume = make(chan struct{})
		metrics.CountByReason("CommWriteAdmissionPauseTransition", lane, "paused", 1)
		parts := strings.SplitN(lane, ":", 2)
		metrics.GaugeByChannel("CommWriteAdmissionPaused", "Count", parts[0], parts[1], 1)
		utils.Warn(fmt.Sprintf("commwrite admission paused lane=%s occupancy_percent=%d", lane, percent))
	}
	if state.paused && percent <= 60 {
		state.paused = false
		pausedFor := time.Since(state.pausedAt)
		close(state.resume)
		metrics.CountByReason("CommWriteAdmissionPauseTransition", lane, "resumed", 1)
		parts := strings.SplitN(lane, ":", 2)
		metrics.GaugeByChannel("CommWriteAdmissionPaused", "Count", parts[0], parts[1], 0)
		metrics.ValueByChannel("CommWriteAdmissionPauseMilliseconds", "Milliseconds", parts[0], parts[1], float64(pausedFor.Milliseconds()))
		utils.Info(fmt.Sprintf("commwrite admission resumed lane=%s occupancy_percent=%d", lane, percent))
	}
	paused := state.paused
	p.mu.Unlock()
	if paused {
		return ErrStreamHighWater
	}

	return nil
}

func (p *Producer) resumeSignal(stream string) <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.states[stream]
	if state == nil || !state.paused {
		ready := make(chan struct{})
		close(ready)
		return ready
	}
	return state.resume
}

func admissionReason(err error) string {
	if errors.Is(err, ErrStreamHighWater) {
		return "high_water"
	}
	return "redis"
}

func StreamKey(client, channel string) string {
	return streamPrefix(channelKey(client, channel)) + ":write"
}

func streamPrefix(lane string) string { return "commwrite:{" + lane + "}" }

func channelKey(client, channel string) string {
	return strings.ToLower(strings.TrimSpace(client)) + ":" + strings.ToLower(strings.TrimSpace(channel))
}

func channelKeyFromEntry(entry string) string {
	parts := strings.Split(strings.TrimSpace(entry), ":")
	if len(parts) != 2 {
		return ""
	}

	return channelKey(parts[0], parts[1])
}

func streamBudget(client, channel string) int64 {
	want := channelKey(client, channel)
	for _, entry := range strings.Split(config.Configs.CommWriteStreamBudgetOverrides, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) != 3 || channelKey(parts[0], parts[1]) != want {
			continue
		}

		budget, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		if err == nil && budget > 0 {
			return budget
		}
	}

	return 0
}

var supportedChannels = map[string]string{
	"wecredit:sms":      MapperMarketingSMS,
	"wecredit:whatsapp": MapperMarketingWhatsapp,
}
