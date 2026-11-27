package policy

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/sdk/variables"
)

const (
	// DefaultZapCashSendCutoffHourIST is 20:00 IST (8 PM). Product hard stop for ZapCash vendor calls.
	DefaultZapCashSendCutoffHourIST = 20
	// ZapCashSendCutoffDisabledHour disables the ZapCash send cutoff when hour is this sentinel.
	ZapCashSendCutoffDisabledHour = -1

	DecisionZapCashSendCutoff = "ZAPCASH_SEND_CUTOFF"
)

const ZapCashSendCutoffMessageFmt = "blocked after %d IST"

// ParseZapCashSendCutoffHourIST reads ZAPCASH_SEND_CUTOFF_HOUR_IST.
// Blank or non-numeric → 20. Explicit -1 disables. Values outside 0–23 (except -1) fall back to 20.
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

// EvaluateZapCashSendCutoff blocks ZapCash vendor sends at/after cutoffHour IST.
// Non-ZapCash clients are always allowed. cutoffHour < 0 disables the gate.
func EvaluateZapCashSendCutoff(client string, now time.Time, cutoffHour int) Decision {
	currentIST := IST(now)
	if !strings.EqualFold(strings.TrimSpace(client), variables.ZapCash) {
		return Decision{Code: DecisionAllowed, CurrentIST: currentIST}
	}

	if cutoffHour < 0 {
		return Decision{Code: DecisionAllowed, CurrentIST: currentIST}
	}

	hour := cutoffHour
	if hour > 23 {
		hour = DefaultZapCashSendCutoffHourIST
	}

	if currentIST.Hour() >= hour {
		return Decision{
			Code:       DecisionZapCashSendCutoff,
			Message:    fmt.Sprintf(ZapCashSendCutoffMessageFmt, hour),
			CurrentIST: currentIST,
		}
	}

	return Decision{Code: DecisionAllowed, CurrentIST: currentIST}
}
