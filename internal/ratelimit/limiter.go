package ratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// TokenBucket is a simple in-process limiter (no external deps).
type TokenBucket struct {
	mu         sync.Mutex
	ratePerSec float64
	burst      float64
	tokens     float64
	last       time.Time
	zeroRate   bool
}

func newTokenBucket(rps float64) *TokenBucket {
	zeroRate := rps <= 0
	if zeroRate {
		rps = 0
	}
	burst := rps
	if burst < 1 {
		burst = 1
	}
	return &TokenBucket{
		ratePerSec: rps,
		burst:      burst,
		tokens:     burst,
		last:       time.Now(),
		zeroRate:   zeroRate,
	}
}

// Wait blocks until one token is available or ctx is cancelled.
func (b *TokenBucket) Wait(ctx context.Context) error {
	for {
		if b.zeroRate {
			<-ctx.Done()
			return ctx.Err()
		}

		b.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.last).Seconds()
		b.last = now
		b.tokens += elapsed * b.ratePerSec
		if b.tokens > b.burst {
			b.tokens = b.burst
		}
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		need := 1 - b.tokens
		wait := time.Duration(need / b.ratePerSec * float64(time.Second))
		b.mu.Unlock()
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// redisTakeScript is a shared client-total token bucket.
// ARGV: rate (tokens/sec), burst (~1s of tokens), now_ms.
// Returns 0 when a token is taken, or wait time in ms.
// A missing key starts at burst (not unlimited), then consumes one token.
var redisTakeScript = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
if rate == nil or rate <= 0 or burst == nil or burst < 1 or now_ms == nil then
  return -1
end

local data = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts = tonumber(data[2])

if tokens == nil or ts == nil then
  tokens = burst
  ts = now_ms
end

-- Never move the shared clock backward when a task's clock is behind.
if now_ms < ts then
  now_ms = ts
end
local elapsed = (now_ms - ts) / 1000.0
if elapsed < 0 then
  elapsed = 0
end
tokens = tokens + elapsed * rate
if tokens > burst then
  tokens = burst
end
ts = now_ms

if tokens >= 1 then
  tokens = tokens - 1
  redis.call('HSET', key, 'tokens', tostring(tokens), 'ts', tostring(ts))
  local ttl_ms = math.ceil((burst / rate) * 2000)
  if ttl_ms < 2000 then
    ttl_ms = 2000
  end
  redis.call('PEXPIRE', key, ttl_ms)
  return 0
end

local need = 1 - tokens
local wait_ms = math.ceil((need / rate) * 1000)
if wait_ms < 1 then
  wait_ms = 1
end
redis.call('HSET', key, 'tokens', tostring(tokens), 'ts', tostring(ts))
local ttl_ms = math.ceil((burst / rate) * 2000)
if ttl_ms < 2000 then
  ttl_ms = 2000
end
redis.call('PEXPIRE', key, ttl_ms)
return wait_ms
`)

var (
	fallbackMu       sync.Mutex
	fallbackRegistry = map[string]*TokenBucket{}

	overridesMu       sync.RWMutex
	lastGoodOverrides map[string]float64
	lastGoodRaw       string
	defaultRPS        = 10.0
	approvedCapsRaw   string
	taskSlots         = 1

	redisMu     sync.RWMutex
	redisClient *redis.Client
)

// ConfigureRedis enables the shared Redis client budget. slots must equal ECS autoscaling max.
// A nil client disables Redis and uses the in-process bucket at the full client-total RPS.
func ConfigureRedis(rdb *redis.Client, slots int) {
	redisMu.Lock()
	redisClient = rdb
	redisMu.Unlock()

	overridesMu.Lock()
	if slots < 1 {
		slots = 1
	}
	taskSlots = slots
	overridesMu.Unlock()

	fallbackMu.Lock()
	fallbackRegistry = map[string]*TokenBucket{}
	fallbackMu.Unlock()
}

// TaskSlots returns the configured PROVIDER_RPS_TASK_SLOTS value (tests/ops).
func TaskSlots() int {
	overridesMu.RLock()
	defer overridesMu.RUnlock()
	return taskSlots
}

// Key builds a limiter identity from vendor and client (legacy, no channel).
func Key(vendor, client string) string {
	return KeyWithChannel(vendor, client, "")
}

// KeyWithChannel builds vendor:client or vendor:client:channel (lowercased).
func KeyWithChannel(vendor, client, channel string) string {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	client = strings.ToLower(strings.TrimSpace(client))
	channel = strings.ToLower(strings.TrimSpace(channel))
	if vendor == "" {
		vendor = "default"
	}
	if client == "" {
		client = "default"
	}
	if channel == "" {
		return vendor + ":" + client
	}
	return vendor + ":" + client + ":" + channel
}

func redisKey(limitKey string) string {
	return "provider_rps:" + limitKey
}

// WaitFor acquires one permit for key (e.g. "sinch:wecredit:whatsapp").
// With Redis configured, the budget is the client total across tasks. On Redis
// errors, falls back to a local bucket at floor(client_total / TASK_SLOTS).
func WaitFor(ctx context.Context, key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		key = "default"
	}

	redisMu.RLock()
	rdb := redisClient
	redisMu.RUnlock()
	if rdb != nil {
		if err := waitRedis(ctx, rdb, key); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return ctx.Err()
		}
		// Redis error (or script failure): do not send unlimited.
		return getFallbackBucket(key).Wait(ctx)
	}
	// Missing Redis is the same safety condition as an unavailable Redis server:
	// divide the client total by the configured task slots on this process.
	return getFallbackBucket(key).Wait(ctx)
}

func waitRedis(ctx context.Context, rdb *redis.Client, key string) error {
	rate := rpsForKey(key)
	burst := rate
	if burst < 1 {
		burst = 1
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		waitMs, err := takeRedis(ctx, rdb, key, rate, burst)
		if err != nil {
			return err
		}

		if waitMs <= 0 {
			return nil
		}

		wait := time.Duration(waitMs) * time.Millisecond
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// One local sleep per denied take — no busy-polling Redis.
		}
	}
}

func takeRedis(ctx context.Context, rdb *redis.Client, key string, rate, burst float64) (int64, error) {
	nowMs := time.Now().UnixMilli()
	res, err := redisTakeScript.Run(ctx, rdb, []string{redisKey(key)}, rate, burst, nowMs).Int64()
	if err != nil {
		return 0, err
	}
	if res < 0 {
		return 0, fmt.Errorf("provider_rps_redis_script_invalid_args")
	}
	return res, nil
}

func getFallbackBucket(key string) *TokenBucket {
	fallbackMu.Lock()
	defer fallbackMu.Unlock()
	if b, ok := fallbackRegistry[key]; ok {
		return b
	}
	b := newTokenBucket(localFallbackRPS(key))
	fallbackRegistry[key] = b
	return b
}

func localFallbackRPS(key string) float64 {
	overridesMu.RLock()
	slots := taskSlots
	overridesMu.RUnlock()
	if slots < 1 {
		slots = 1
	}
	total := rpsForKey(key)
	return math.Floor(total / float64(slots))
}

func rpsForKey(key string) float64 {
	overridesMu.RLock()
	overrides := lastGoodOverrides
	def := defaultRPS
	overridesMu.RUnlock()

	if rps, ok := lookupWithFallback(overrides, key); ok {
		return rps
	}
	return def
}

// lookupWithFallback tries key, then parent prefixes:
// vendor:client:channel → vendor:client → vendor.
func lookupWithFallback(m map[string]float64, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return 0, false
	}
	if rps, ok := m[key]; ok {
		return rps, true
	}
	for {
		i := strings.LastIndexByte(key, ':')
		if i <= 0 {
			break
		}
		key = key[:i]
		if rps, ok := m[key]; ok {
			return rps, true
		}
	}
	return 0, false
}

// ParseOverrides parses comma-separated vendor[:client[:channel]]:rps entries.
func ParseOverrides(raw string) map[string]float64 {
	out := map[string]float64{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.Split(part, ":")
		if len(pieces) < 2 {
			continue
		}
		rps, err := strconv.ParseFloat(strings.TrimSpace(pieces[len(pieces)-1]), 64)
		if err != nil || rps <= 0 || math.IsNaN(rps) || math.IsInf(rps, 0) {
			continue
		}
		name := strings.ToLower(strings.Join(pieces[:len(pieces)-1], ":"))
		out[name] = rps
	}
	return out
}

// ValidateOverridesAgainstCaps ensures every override ≤ matching approved cap
// (exact key, then vendor:client, then vendor). Empty caps skips validation.
func ValidateOverridesAgainstCaps(overridesRaw, capsRaw string) error {
	if err := validateOverrideEntries(overridesRaw, "overrides"); err != nil {
		return err
	}

	if err := validateOverrideEntries(capsRaw, "approved caps"); err != nil {
		return err
	}
	overrides := ParseOverrides(overridesRaw)
	caps := ParseOverrides(capsRaw)
	if len(caps) == 0 {
		return nil
	}
	for key, rps := range overrides {
		cap, ok := lookupWithFallback(caps, key)
		if !ok {
			return fmt.Errorf("provider_rps_overrides_exceed_cap: no APPROVED_CAPS entry for %q", key)
		}
		if rps > cap {
			return fmt.Errorf("provider_rps_overrides_exceed_cap: %s=%.0f exceeds approved cap %.0f", key, rps, cap)
		}
	}
	return nil
}

func validateOverrideEntries(raw, label string) error {
	for _, part := range strings.Split(strings.TrimSpace(raw), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		pieces := strings.Split(part, ":")
		if len(pieces) < 2 || len(pieces) > 4 || strings.TrimSpace(strings.Join(pieces[:len(pieces)-1], ":")) == "" {
			return fmt.Errorf("invalid provider rate limit %s entry %q", label, part)
		}

		for _, keyPart := range pieces[:len(pieces)-1] {
			if strings.TrimSpace(keyPart) == "" {
				return fmt.Errorf("invalid provider rate limit %s entry %q", label, part)
			}
		}

		rps, err := strconv.ParseFloat(strings.TrimSpace(pieces[len(pieces)-1]), 64)
		if err != nil || rps <= 0 || math.IsNaN(rps) || math.IsInf(rps, 0) {
			return fmt.Errorf("invalid provider rate limit %s entry %q", label, part)
		}
	}

	return nil
}

// InitFromConfig validates overrides against caps and installs last-good map.
// Call once at process startup; returns error for fail-fast boot.
// taskSlotsRaw is PROVIDER_RPS_TASK_SLOTS (autoscaling max); blank or invalid → 1.
func InitFromConfig(overridesRaw, capsRaw, defaultRaw, taskSlotsRaw string) error {
	if err := ValidateOverridesAgainstCaps(overridesRaw, capsRaw); err != nil {
		return err
	}

	slots := int(positiveFloat(taskSlotsRaw, 1))
	if slots < 1 {
		slots = 1
	}
	overridesMu.Lock()
	defer overridesMu.Unlock()
	lastGoodOverrides = ParseOverrides(overridesRaw)
	lastGoodRaw = overridesRaw
	approvedCapsRaw = capsRaw
	defaultRPS = positiveFloat(defaultRaw, 10)
	taskSlots = slots
	return nil
}

// TryApplyOverrides validates a new overrides string against caps (or last caps).
// On success, replaces last-good and clears buckets. On failure, keeps last-good.
func TryApplyOverrides(overridesRaw, capsRaw string) error {
	if strings.TrimSpace(capsRaw) == "" {
		overridesMu.RLock()
		capsRaw = approvedCapsRaw
		overridesMu.RUnlock()
	}
	if err := ValidateOverridesAgainstCaps(overridesRaw, capsRaw); err != nil {
		return err
	}
	overridesMu.Lock()
	lastGoodOverrides = ParseOverrides(overridesRaw)
	lastGoodRaw = overridesRaw
	if strings.TrimSpace(capsRaw) != "" {
		approvedCapsRaw = capsRaw
	}
	overridesMu.Unlock()

	fallbackMu.Lock()
	fallbackRegistry = map[string]*TokenBucket{}
	fallbackMu.Unlock()
	return nil
}

// LastGoodOverridesRaw returns the last validated overrides string (tests/ops).
func LastGoodOverridesRaw() string {
	overridesMu.RLock()
	defer overridesMu.RUnlock()
	return lastGoodRaw
}

// ResolvedRPS returns the effective client-total RPS for a key (tests / cross-client checks).
func ResolvedRPS(key string) float64 {
	return rpsForKey(strings.ToLower(strings.TrimSpace(key)))
}

// LocalFallbackRPS returns floor(client_total / TASK_SLOTS) used when Redis errors.
func LocalFallbackRPS(key string) float64 {
	return localFallbackRPS(strings.ToLower(strings.TrimSpace(key)))
}

func positiveFloat(raw string, fallback float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	return v
}

// ResetForTest clears limiter state (unit tests only).
func ResetForTest() {
	fallbackMu.Lock()
	fallbackRegistry = map[string]*TokenBucket{}
	fallbackMu.Unlock()
	overridesMu.Lock()
	lastGoodOverrides = nil
	lastGoodRaw = ""
	approvedCapsRaw = ""
	defaultRPS = 10
	taskSlots = 1
	overridesMu.Unlock()
	redisMu.Lock()
	redisClient = nil
	redisMu.Unlock()
}
