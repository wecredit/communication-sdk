package fcm

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"
)

const (
	maxProviderAttempts = 2
	maxRetryJitter      = 2 * time.Second
)

type attemptSender interface {
	Send(ctx context.Context, client string, payload SendRequest) (SendResponse, error)
}

// RetryGuard revalidates claim ownership and any available eligibility
// identity immediately before the second provider attempt.
type RetryGuard func(ctx context.Context) (current bool, err error)

// AttemptObserver runs immediately before each provider call. Durable callers
// use it to persist the attempt count before an in-flight worker can exit.
type AttemptObserver func(ctx context.Context, attempt int) error

type ExecutionResult struct {
	Outcome      Outcome
	Code         string
	MessageID    string
	AttemptCount int
}

type RetryExecutor struct {
	sender attemptSender
	jitter func(time.Duration) time.Duration
	wait   func(context.Context, time.Duration) error
}

func NewRetryExecutor(sender attemptSender) (*RetryExecutor, error) {
	if sender == nil {
		return nil, errors.New("FCM attempt sender is required")
	}

	return &RetryExecutor{
		sender: sender,
		jitter: fullJitter,
		wait:   waitForRetry,
	}, nil
}

// Execute performs one initial FCM attempt and at most one retry. The executor
// owns no durable state; its result is intended to be persisted atomically by
// the dispatch-ledger layer added separately.
func (e *RetryExecutor) Execute(ctx context.Context, client string, payload SendRequest, guard RetryGuard) (ExecutionResult, error) {
	return e.ExecuteWithObserver(ctx, client, payload, guard, nil)
}

// ExecuteWithObserver performs the same bounded retry policy as Execute and
// invokes observer before each provider call. An observer failure prevents the
// provider call so durable attempt state cannot lag behind an actual send.
func (e *RetryExecutor) ExecuteWithObserver(
	ctx context.Context,
	client string,
	payload SendRequest,
	guard RetryGuard,
	observer AttemptObserver,
) (ExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ExecutionResult{}, err
	}

	for attempt := 1; attempt <= maxProviderAttempts; attempt++ {
		if attempt > 1 {
			if err := e.wait(ctx, e.jitter(maxRetryJitter)); err != nil {
				return ExecutionResult{Outcome: OutcomeRetryable, Code: "RETRY_WAIT_CANCELLED", AttemptCount: attempt - 1}, err
			}

			if guard != nil {
				current, err := guard(ctx)
				if err != nil {
					return ExecutionResult{Outcome: OutcomeRetryable, Code: "RETRY_GUARD_ERROR", AttemptCount: attempt - 1}, err
				}

				if !current {
					return ExecutionResult{Outcome: OutcomeCancelledStale, Code: "STALE_BEFORE_RETRY", AttemptCount: attempt - 1}, nil
				}
			}
		}

		if observer != nil {
			if err := observer(ctx, attempt); err != nil {
				return ExecutionResult{
					Outcome:      OutcomeRetryable,
					Code:         "ATTEMPT_RECORD_ERROR",
					AttemptCount: attempt - 1,
				}, err
			}
		}

		response, sendErr := e.sender.Send(ctx, client, payload)
		classified := Classify(response, sendErr)
		result := ExecutionResult{
			Outcome:      classified.Outcome,
			Code:         classified.Code,
			MessageID:    response.MessageID,
			AttemptCount: attempt,
		}

		switch classified.Outcome {
		case OutcomeSubmitted, OutcomeFailedFinal, OutcomeCancelledStale:
			return result, nil
		case OutcomeRetryable:
			if attempt == maxProviderAttempts {
				result.Outcome = OutcomeFailedFinal
				return result, nil
			}
		default:
			result.Outcome = OutcomeFailedFinal
			result.Code = "INVALID_CLASSIFICATION"
			return result, nil
		}
	}

	return ExecutionResult{Outcome: OutcomeFailedFinal, Code: "ATTEMPTS_EXHAUSTED", AttemptCount: maxProviderAttempts}, nil
}

func fullJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}

	random, err := rand.Int(rand.Reader, big.NewInt(int64(max)+1))
	if err != nil {
		return 0
	}

	return time.Duration(random.Int64())
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
