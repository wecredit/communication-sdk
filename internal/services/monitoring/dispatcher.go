package monitoring

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wecredit/communication-sdk/internal/metrics"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

const (
	defaultWorkers = 2
	defaultBuffer  = 100
)

type Dispatcher struct {
	config    RuntimeConfig
	jobs      chan AcceptedResult
	processor func(RuntimeConfig, AcceptedResult)
	dropped   atomic.Uint64
}

var (
	dispatcherMu sync.RWMutex
	active       *Dispatcher
)

// Init enables monitoring only when the master switch and complete configuration are valid.
func Init() {
	dispatcherMu.Lock()
	active = nil
	dispatcherMu.Unlock()
	cfg := loadRuntimeConfig()
	if !cfg.Enabled {
		utils.Info("ZapCash monitoring disabled")
		return
	}
	d := NewDispatcher(cfg, defaultWorkers, defaultBuffer, processAcceptedResult)
	dispatcherMu.Lock()
	active = d
	dispatcherMu.Unlock()
}

// NewDispatcher exposes the bounded worker boundary for black-box tests under test/.
func NewDispatcher(cfg RuntimeConfig, workers, buffer int, processor func(RuntimeConfig, AcceptedResult)) *Dispatcher {
	d := &Dispatcher{config: cfg, jobs: make(chan AcceptedResult, buffer), processor: processor}
	for i := 0; i < workers; i++ {
		go d.worker()
	}
	return d
}

func (d *Dispatcher) worker() {
	for job := range d.jobs {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					metrics.Count("zapcash_monitor_panic_recovered_total", job.ResolvedVendor, "zapcash", 1)
					utils.Error(fmt.Errorf("ZapCash monitoring panic recovered channel=%s stage=%.2f: %v",
						job.Payload.Channel, job.Payload.Stage, recovered))
				}
			}()

			metrics.Count("zapcash_monitor_job_submitted_total", job.ResolvedVendor, "zapcash", 1)
			if dropped := d.dropped.Swap(0); dropped > 0 {
				metrics.Count("zapcash_monitor_dispatcher_saturated_total", job.ResolvedVendor, "zapcash", int(dropped))
			}
			d.processor(d.config, job)
		}()
	}
}

// Submit places a job without blocking and returns false when the bounded buffer is full.
func (d *Dispatcher) Submit(result AcceptedResult) bool {
	select {
	case d.jobs <- result:
		return true
	default:
		d.dropped.Add(1)
		return false
	}
}

// TrySubmit is deliberately non-blocking and never returns an error to production handlers.
func TrySubmit(result AcceptedResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			metrics.Count("zapcash_monitor_submit_panic_recovered_total", result.ResolvedVendor, "zapcash", 1)
			utils.Error(fmt.Errorf("ZapCash monitoring submission panic recovered channel=%s stage=%.2f: %v",
				result.Payload.Channel, result.Payload.Stage, recovered))
		}
	}()

	if result.Payload.IsMonitorCopy || !strings.EqualFold(strings.TrimSpace(result.Payload.Client), "zapcash") {
		return
	}

	switch strings.ToUpper(strings.TrimSpace(result.Payload.Channel)) {
	case "SMS", "RCS", "WHATSAPP":
	default:
		return
	}
	dispatcherMu.RLock()
	d := active
	dispatcherMu.RUnlock()

	if d == nil {
		utils.Warn(fmt.Sprintf("ZapCash monitoring submit skipped because dispatcher is nil channel=%s stage=%.2f vendor=%s client=%s",
			result.Payload.Channel, result.Payload.Stage, result.ResolvedVendor, result.Payload.Client))
		return
	}

	utils.Info(fmt.Sprintf("ZapCash monitoring queued channel=%s stage=%.2f vendor=%s template=%s client=%s mobile=%s",
		result.Payload.Channel, result.Payload.Stage, result.ResolvedVendor, result.ResolvedTemplate, result.Payload.Client, result.Payload.Mobile))
	d.Submit(result)
}
