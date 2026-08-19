package ratelimit

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wecredit/communication-sdk/config"
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
)

// Key builds a limiter identity from vendor and client (credential proxy).
func Key(vendor, client string) string {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	client = strings.ToLower(strings.TrimSpace(client))
	if vendor == "" {
		vendor = "default"
	}
	if client == "" {
		client = "default"
	}
	return vendor + ":" + client
}

// WaitFor acquires one permit for key (e.g. "sinch:wecredit").
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
	overrides := ParseOverrides(config.Configs.ProviderRPSOverrides)
	if rps, ok := overrides[key]; ok {
		return rps
	}
	// vendor-only fallback: "sinch:wecredit" -> "sinch"
	if i := strings.IndexByte(key, ':'); i > 0 {
		if rps, ok := overrides[key[:i]]; ok {
			return rps
		}
	}
	return positiveFloat(config.Configs.ProviderRPSDefault, 50)
}

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
		// Formats: vendor:client:rps  OR  vendor:rps
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
	defer registryMu.Unlock()
	registry = map[string]*TokenBucket{}
}
