package outcome

import "testing"

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(Submitted) || !IsTerminal(FailedFinal) || !IsTerminal(AlreadyDone) || !IsTerminal(Skipped) {
		t.Fatal("expected terminal outcomes")
	}
	if IsTerminal(FailedRetryable) || IsTerminal(Unknown) || IsTerminal(Claimed) {
		t.Fatal("expected non-terminal outcomes")
	}
}

func TestClassifyTransportError(t *testing.T) {
	if ClassifyTransportError(errString("i/o timeout")) != Unknown {
		t.Fatal("timeout should be UNKNOWN")
	}
	if ClassifyTransportError(errString("connection refused")) != FailedRetryable {
		t.Fatal("network error should be FAILED_RETRYABLE")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
