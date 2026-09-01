package monitoring_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/services/monitoring"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestDispatcherRecoversPanicAndProcessesNextJob(t *testing.T) {
	var calls atomic.Int32
	completed := make(chan struct{}, 1)
	d := monitoring.NewDispatcher(monitoring.RuntimeConfig{Enabled: true}, 1, 2, func(_ monitoring.RuntimeConfig, _ monitoring.AcceptedResult) {
		if calls.Add(1) == 1 {
			panic("forced monitoring panic")
		}
		completed <- struct{}{}
	})
	d.Submit(monitoring.AcceptedResult{Payload: sdkModels.CommApiRequestBody{Channel: "SMS", Stage: 1}, ResolvedVendor: "PINNACLE"})
	d.Submit(monitoring.AcceptedResult{Payload: sdkModels.CommApiRequestBody{Channel: "SMS", Stage: 2}, ResolvedVendor: "PINNACLE"})
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("dispatcher worker did not survive a monitoring panic")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected both jobs to run, got %d", calls.Load())
	}
}

func TestDispatcherSubmitIsNonBlockingWhenSaturated(t *testing.T) {
	d := monitoring.NewDispatcher(monitoring.RuntimeConfig{Enabled: true}, 0, 1, func(monitoring.RuntimeConfig, monitoring.AcceptedResult) {})
	if !d.Submit(monitoring.AcceptedResult{}) {
		t.Fatal("first job should fit in the buffer")
	}
	start := time.Now()
	if d.Submit(monitoring.AcceptedResult{}) {
		t.Fatal("second job should be rejected when buffer is full")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("saturated submission blocked")
	}
}
