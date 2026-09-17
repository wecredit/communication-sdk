package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TokenBucket is a simple in-process limiter (no external deps).
type TokenBucket struct {
	mu         sync.Mutex
	ratePerSec float64
	burst      float64
	tokens     float64
	last       time.Time
}

func newTokenBucket(rps float64) *TokenBucket {
	if rps <= 0 {
		rps = 50
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
	}
}

// Wait blocks until one token is available or ctx is cancelled.
func (b *TokenBucket) Wait(ctx context.Context) error {
	for {
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

var (
	registryMu sync.Mutex
	registry   = map[string]*TokenBucket{}

	overridesMu       sync.RWMutex
	lastGoodOverrides map[string]float64
	lastGoodRaw       string
	defaultRPS        = 50.0
	approvedCapsRaw   string
)

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

// WaitFor acquires one permit for key (e.g. "sinch:wecredit:whatsapp").
func WaitFor(ctx context.Context, key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		key = "default"
	}
	bucket := getBucket(key)
	return bucket.Wait(ctx)
}

func getBucket(key string) *TokenBucket {
	registryMu.Lock()
	defer registryMu.Unlock()
	if b, ok := registry[key]; ok {
		return b
	}
	b := newTokenBucket(rpsForKey(key))
	registry[key] = b
	return b
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
		if err != nil || rps <= 0 {
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

// InitFromConfig validates overrides against caps and installs last-good map.
// Call once at process startup; returns error for fail-fast boot.
func InitFromConfig(overridesRaw, capsRaw, defaultRaw string) error {
	if err := ValidateOverridesAgainstCaps(overridesRaw, capsRaw); err != nil {
		return err
	}
	overridesMu.Lock()
	defer overridesMu.Unlock()
	lastGoodOverrides = ParseOverrides(overridesRaw)
	lastGoodRaw = overridesRaw
	approvedCapsRaw = capsRaw
	defaultRPS = positiveFloat(defaultRaw, 50)
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

	registryMu.Lock()
	registry = map[string]*TokenBucket{}
	registryMu.Unlock()
	return nil
}

// LastGoodOverridesRaw returns the last validated overrides string (tests/ops).
func LastGoodOverridesRaw() string {
	overridesMu.RLock()
	defer overridesMu.RUnlock()
	return lastGoodRaw
}

// ResolvedRPS returns the effective RPS for a key (tests / cross-client checks).
func ResolvedRPS(key string) float64 {
	return rpsForKey(strings.ToLower(strings.TrimSpace(key)))
}

func positiveFloat(raw string, fallback float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

// ResetForTest clears limiter state (unit tests only).
func ResetForTest() {
	registryMu.Lock()
	registry = map[string]*TokenBucket{}
	registryMu.Unlock()
	overridesMu.Lock()
	lastGoodOverrides = nil
	lastGoodRaw = ""
	approvedCapsRaw = ""
	defaultRPS = 50
	overridesMu.Unlock()
}
