package policy

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/sdk/variables"
)

const (
	// DefaultZapCashSendCutoffHourIST is 21:00 IST (9 PM). Product hard stop for ZapCash vendor calls.
	DefaultZapCashSendCutoffHourIST = 21
	// DefaultZapCashSendOpenHourIST is 08:00 IST. Outside [open, cutoff) leftover queue
	// messages must not dial (overnight retries are rejected).
	DefaultZapCashSendOpenHourIST = 8
	// ZapCashSendCutoffDisabledHour disables the ZapCash send cutoff when hour is this sentinel.
	ZapCashSendCutoffDisabledHour = -1
	// ZapCashSendOpenDisabledHour disables the morning open bound (cutoff-only mode).
	ZapCashSendOpenDisabledHour = -1

	DecisionZapCashSendCutoff = "ZAPCASH_SEND_CUTOFF"
)

const (
	ZapCashSendCutoffMessageFmt    = "blocked after %d IST"
	ZapCashSendBeforeOpenMessageFmt = "blocked before %d IST"
)

// ParseZapCashSendCutoffHourIST reads ZAPCASH_SEND_CUTOFF_HOUR_IST.
// Blank or non-numeric → 21. Explicit -1 disables. Values outside 0–23 (except -1) fall back to 21.
func ParseZapCashSendCutoffHourIST(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultZapCashSendCutoffHourIST
	}
	hour, err := strconv.Atoi(trimmed)
	if err != nil {
		return DefaultZapCashSendCutoffHourIST
	}
	if hour == ZapCashSendCutoffDisabledHour {
		return ZapCashSendCutoffDisabledHour
	}
	if hour < 0 || hour > 23 {
		return DefaultZapCashSendCutoffHourIST
	}
	return hour
}

// ParseZapCashSendOpenHourIST reads ZAPCASH_SEND_OPEN_HOUR_IST.
// Blank or non-numeric → 8. Explicit -1 disables the morning bound. Values outside 0–23 (except -1) fall back to 8.
func ParseZapCashSendOpenHourIST(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultZapCashSendOpenHourIST
	}
	hour, err := strconv.Atoi(trimmed)
	if err != nil {
		return DefaultZapCashSendOpenHourIST
	}
	if hour == ZapCashSendOpenDisabledHour {
		return ZapCashSendOpenDisabledHour
	}
	if hour < 0 || hour > 23 {
		return DefaultZapCashSendOpenHourIST
	}
	return hour
}

// EvaluateZapCashSendCutoff blocks ZapCash vendor sends outside [openHour, cutoffHour) IST.
// Non-ZapCash clients are always allowed. cutoffHour < 0 disables the gate entirely.
// openHour < 0 disables only the morning bound (legacy cutoff-only behavior).
func EvaluateZapCashSendCutoff(client string, now time.Time, openHour, cutoffHour int) Decision {
	currentIST := IST(now)
	if !strings.EqualFold(strings.TrimSpace(client), variables.ZapCash) {
		return Decision{Code: DecisionAllowed, CurrentIST: currentIST}
	}

	if cutoffHour < 0 {
		return Decision{Code: DecisionAllowed, CurrentIST: currentIST}
	}

	cutoff := cutoffHour
	if cutoff > 23 {
		cutoff = DefaultZapCashSendCutoffHourIST
	}

	open := openHour
	if open > 23 {
		open = DefaultZapCashSendOpenHourIST
	}

	hour := currentIST.Hour()
	if open >= 0 && hour < open {
		return Decision{
			Code:       DecisionZapCashSendCutoff,
			Message:    fmt.Sprintf(ZapCashSendBeforeOpenMessageFmt, open),
			CurrentIST: currentIST,
		}
	}
	if hour >= cutoff {
		return Decision{
			Code:       DecisionZapCashSendCutoff,
			Message:    fmt.Sprintf(ZapCashSendCutoffMessageFmt, cutoff),
			CurrentIST: currentIST,
		}
	}

	return Decision{Code: DecisionAllowed, CurrentIST: currentIST}
}
