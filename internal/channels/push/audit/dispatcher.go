package audit

import (
	"errors"
	"fmt"
	"sync/atomic"
)

const (
	defaultWorkers = 2
	defaultBuffer  = 100
)

// Job contains exactly one audit record. Keeping input and output writes on the
// same bounded queue prevents audit persistence from blocking PUSH delivery.
type Job struct {
	Input  *Input
	Output *Output
}

// Processor persists an audit job. Database-specific writing is deliberately
// injected so the dispatcher does not own connections, table names, or schema.
type Processor func(Job) error

// ErrorHandler observes asynchronous persistence failures and recovered worker
// panics. Implementations must not log complete device tokens or credentials.
type ErrorHandler func(error)

// Dispatcher is a bounded, fire-and-forget worker pool for PUSH audits.
type Dispatcher struct {
	jobs      chan Job
	processor Processor
	onError   ErrorHandler
	dropped   atomic.Uint64
}

// NewDispatcher starts a PUSH audit dispatcher. Non-positive worker and buffer
// values use the package defaults.
func NewDispatcher(workers, buffer int, processor Processor, onError ErrorHandler) (*Dispatcher, error) {
	if processor == nil {
		return nil, errors.New("PUSH audit processor is required")
	}

	if workers <= 0 {
		workers = defaultWorkers
	}

	if buffer <= 0 {
		buffer = defaultBuffer
	}

	dispatcher := &Dispatcher{
		jobs:      make(chan Job, buffer),
		processor: processor,
		onError:   onError,
	}
	for worker := 0; worker < workers; worker++ {
		go dispatcher.runWorker()
	}

	return dispatcher, nil
}

func (d *Dispatcher) runWorker() {
	for job := range d.jobs {
		d.process(job)
	}
}

func (d *Dispatcher) process(job Job) {
	defer func() {
		if recovered := recover(); recovered != nil {
			d.report(fmt.Errorf("PUSH audit worker panic recovered: %v", recovered))
		}
	}()

	if err := d.processor(job); err != nil {
		d.report(fmt.Errorf("persist PUSH audit: %w", err))
	}
}

func (d *Dispatcher) report(err error) {
	if d.onError != nil {
		d.onError(err)
	}
}

// TrySubmit queues an audit without blocking. It returns false for malformed
// jobs or when the bounded queue is full; PUSH delivery should continue.
func (d *Dispatcher) TrySubmit(job Job) bool {
	if d == nil || !validJob(job) {
		return false
	}

	select {
	case d.jobs <- job:
		return true
	default:
		d.dropped.Add(1)
		return false
	}
}

// Dropped returns the number of jobs rejected because the queue was full.
func (d *Dispatcher) Dropped() uint64 {
	if d == nil {
		return 0
	}
	return d.dropped.Load()
}

func validJob(job Job) bool {
	return (job.Input != nil) != (job.Output != nil)
}
