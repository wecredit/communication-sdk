// Package vendormetrics records per-vendor WhatsApp send counts and latency
// so multi-vendor starvation and client-total RPS can be verified in prod.
package vendormetrics

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wecredit/communication-sdk/sdk/utils"
)

const logEvery = int64(100)

type vendorStats struct {
	sends      atomic.Int64
	accepted   atomic.Int64
	latencyNs  atomic.Int64
	loggedAt   atomic.Int64 // send count when last logged
}

var (
	mu    sync.Mutex
	stats = map[string]*vendorStats{}
)

func forVendor(vendor string) *vendorStats {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	if vendor == "" {
		vendor = "unknown"
	}
	mu.Lock()
	defer mu.Unlock()
	if s, ok := stats[vendor]; ok {
		return s
	}
	s := &vendorStats{}
	stats[vendor] = s
	return s
}

// Record increments send counters for vendor and periodically logs aggregates (no PII).
func Record(vendor string, latency time.Duration, accepted bool) {
	s := forVendor(vendor)
	n := s.sends.Add(1)
	if accepted {
		s.accepted.Add(1)
	}
	if latency > 0 {
		s.latencyNs.Add(latency.Nanoseconds())
	}

	prev := s.loggedAt.Load()
	if n-prev < logEvery {
		return
	}
	if !s.loggedAt.CompareAndSwap(prev, n) {
		return
	}

	totalLatency := s.latencyNs.Load()
	avgMs := float64(0)
	if n > 0 {
		avgMs = float64(totalLatency) / float64(n) / float64(time.Millisecond)
	}
	utils.Info(fmt.Sprintf(
		"metric_name=whatsapp_vendor_send vendor=%s sends=%d accepted=%d avg_latency_ms=%.1f",
		strings.ToLower(strings.TrimSpace(vendor)),
		n,
		s.accepted.Load(),
		avgMs,
	))
}

// SnapshotForTest returns cumulative send count for vendor (tests only).
func SnapshotForTest(vendor string) (sends, accepted int64) {
	s := forVendor(vendor)
	return s.sends.Load(), s.accepted.Load()
}

// ResetForTest clears metrics (tests only).
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	stats = map[string]*vendorStats{}
}
