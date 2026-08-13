package outcome_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
)

func TestIsTerminal(t *testing.T) {
	if !outcome.IsTerminal(outcome.Submitted) || !outcome.IsTerminal(outcome.FailedFinal) || !outcome.IsTerminal(outcome.AlreadyDone) || !outcome.IsTerminal(outcome.Skipped) {
		t.Fatal("expected terminal outcomes")
	}
	if outcome.IsTerminal(outcome.FailedRetryable) || outcome.IsTerminal(outcome.Unknown) || outcome.IsTerminal(outcome.Claimed) {
		t.Fatal("expected non-terminal outcomes")
	}
}

func TestClassifyTransportError(t *testing.T) {
	if outcome.ClassifyTransportError(errString("i/o timeout")) != outcome.Unknown {
		t.Fatal("timeout should be UNKNOWN")
	}
	if outcome.ClassifyTransportError(errString("connection refused")) != outcome.FailedRetryable {
		t.Fatal("network error should be FAILED_RETRYABLE")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
