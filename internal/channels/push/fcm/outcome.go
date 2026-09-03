package fcm

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Outcome string

const (
	OutcomeSubmitted      Outcome = "submitted"
	OutcomeRetryable      Outcome = "retryable"
	OutcomeFailedFinal    Outcome = "failed_final"
	OutcomeCancelledStale Outcome = "cancelled_stale"
)

const (
	errorCodeInvalidArgument  = "INVALID_ARGUMENT"
	errorCodeUnregistered     = "UNREGISTERED"
	errorCodeSenderIDMismatch = "SENDER_ID_MISMATCH"
	errorCodeThirdPartyAuth   = "THIRD_PARTY_AUTH_ERROR"
	errorCodeQuotaExceeded    = "QUOTA_EXCEEDED"
	errorCodeUnavailable      = "UNAVAILABLE"
	errorCodeInternal         = "INTERNAL"
)

// AttemptError lets configuration, authentication, and transport code state
// whether another provider attempt is safe without exposing sensitive values.
type AttemptError struct {
	Outcome Outcome
	Code    string
	Err     error
}

func (e *AttemptError) Error() string {
	if e == nil {
		return ""
	}

	if e.Err != nil {
		return e.Err.Error()
	}

	return e.Code
}

func (e *AttemptError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

func PermanentAttemptError(code string, err error) error {
	return &AttemptError{Outcome: OutcomeFailedFinal, Code: sanitizeErrorCode(code), Err: err}
}

func RetryableAttemptError(code string, err error) error {
	return &AttemptError{Outcome: OutcomeRetryable, Code: sanitizeErrorCode(code), Err: err}
}

type ClassifiedOutcome struct {
	Outcome Outcome
	Code    string
}

// Classify converts one FCM attempt into a ledger-safe outcome. Unknown
// transport errors are retryable; callers should wrap known configuration or
// credential errors with PermanentAttemptError.
func Classify(response SendResponse, err error) ClassifiedOutcome {
	if err != nil {
		var attemptErr *AttemptError
		if errors.As(err, &attemptErr) {
			return ClassifiedOutcome{Outcome: attemptErr.Outcome, Code: sanitizeErrorCode(attemptErr.Code)}
		}

		return ClassifiedOutcome{Outcome: OutcomeRetryable, Code: "TRANSPORT_ERROR"}
	}

	if response.HTTPStatus >= http.StatusOK && response.HTTPStatus < http.StatusMultipleChoices && strings.TrimSpace(response.MessageID) != "" {
		return ClassifiedOutcome{Outcome: OutcomeSubmitted}
	}

	providerCode := strings.ToUpper(strings.TrimSpace(response.ErrorCode))
	statusCode := strings.ToUpper(strings.TrimSpace(response.ErrorStatus))
	code := providerCode
	if code == "" {
		code = statusCode
	}

	if code == "" && response.HTTPStatus > 0 {
		code = "HTTP_" + strconv.Itoa(response.HTTPStatus)
	}

	if code == "" {
		code = "UNKNOWN_RESPONSE"
	}

	switch providerCode {
	case errorCodeUnregistered, errorCodeSenderIDMismatch, errorCodeThirdPartyAuth, errorCodeInvalidArgument:
		return ClassifiedOutcome{Outcome: OutcomeFailedFinal, Code: code}

	case errorCodeQuotaExceeded, errorCodeUnavailable, errorCodeInternal:
		return ClassifiedOutcome{Outcome: OutcomeRetryable, Code: code}
	}

	switch statusCode {
	case errorCodeInvalidArgument, "NOT_FOUND", "PERMISSION_DENIED", "UNAUTHENTICATED":
		return ClassifiedOutcome{Outcome: OutcomeFailedFinal, Code: code}

	case "RESOURCE_EXHAUSTED", errorCodeUnavailable, errorCodeInternal:
		return ClassifiedOutcome{Outcome: OutcomeRetryable, Code: code}
	}

	switch {
	case response.HTTPStatus == http.StatusTooManyRequests:
		return ClassifiedOutcome{Outcome: OutcomeRetryable, Code: code}

	case response.HTTPStatus >= http.StatusInternalServerError:
		return ClassifiedOutcome{Outcome: OutcomeRetryable, Code: code}

	case response.HTTPStatus >= http.StatusBadRequest:
		return ClassifiedOutcome{Outcome: OutcomeFailedFinal, Code: code}

	default:
		return ClassifiedOutcome{Outcome: OutcomeRetryable, Code: code}
	}
}

func sanitizeErrorCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return "UNKNOWN"
	}

	var sanitized strings.Builder
	for _, char := range code {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			sanitized.WriteRune(char)
		}
	}

	if sanitized.Len() == 0 {
		return "UNKNOWN"
	}

	if sanitized.Len() > 64 {
		return sanitized.String()[:64]
	}

	return sanitized.String()
}

func (o ClassifiedOutcome) Validate() error {
	switch o.Outcome {
	case OutcomeSubmitted, OutcomeRetryable, OutcomeFailedFinal, OutcomeCancelledStale:
		return nil
	default:
		return fmt.Errorf("invalid FCM outcome %q", o.Outcome)
	}
}
