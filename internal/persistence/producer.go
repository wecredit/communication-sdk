package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/metrics"
	commredis "github.com/wecredit/communication-sdk/internal/redis"
)

var ErrStreamHighWater = errors.New("commwrite stream is above its pause threshold")

type Producer struct {
	redis   producerRedis
	mu      sync.Mutex
	paused  map[string]bool
	alerted map[string]bool
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

func NewProducer(client producerRedis) *Producer {
	return &Producer{redis: client, paused: make(map[string]bool), alerted: make(map[string]bool)}
}

var defaultProducer = NewProducer(nil)

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
	p.mu.Lock()
	if percent < 50 {
		p.alerted[stream] = false
	}

	if percent >= 50 && !p.alerted[stream] {
		p.alerted[stream] = true
		metrics.CountByReason("CommWriteStreamHighWater", channelKey(client, channel), "alert_50", 1)
	}

	if percent >= 80 {
		p.paused[stream] = true
	}

	paused := p.paused[stream]
	if paused && percent <= 60 {
		p.paused[stream] = false
		paused = false
	}

	p.mu.Unlock()
	if paused {
		metrics.CountByReason("CommWriteStreamHighWater", channelKey(client, channel), "pause_80", 1)
		return ErrStreamHighWater
	}

	return nil
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
