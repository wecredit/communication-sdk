package outcome

import "strings"

// Provider send classification for SMS (and reusable later).
const (
	Claimed         = "CLAIMED"
	Submitted       = "SUBMITTED"
	FailedFinal     = "FAILED_FINAL"
	FailedRetryable = "FAILED_RETRYABLE"
	Unknown         = "UNKNOWN"
	Skipped         = "SKIPPED"
	AlreadyDone     = "ALREADY_DONE"
)

// IsTerminal means SQS may be deleted; non-terminal keeps the message for retry/visibility.
func IsTerminal(status string) bool {
	switch status {
	case Submitted, FailedFinal, Skipped, AlreadyDone:
		return true
	default:
		return false
	}
}

// IsSentDerived maps provider outcome to the legacy IsSent flag for output audits.
func IsSentDerived(status string) bool {
	return status == Submitted || status == AlreadyDone
}

// ClassifyTransportError maps HTTP/client transport failures.
// Timeouts are UNKNOWN (do not blindly retry as success-unknown); other transport issues are retryable.
func ClassifyTransportError(err error) string {
	if err == nil {
		return FailedFinal
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context canceled") {
		return Unknown
	}
	return FailedRetryable
}
