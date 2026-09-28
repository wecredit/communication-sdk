package policy

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	CampaignDateLayout = "2006-01-02" // YYYY-MM-DD is the layout for campaign dates
	CutoffHour         = 20          // 20:00 IST is the cutoff time for sending SMS messages

	DecisionAllowed             = "ALLOWED"                            // The SMS is allowed to be sent
	DecisionCutoff              = "WECREDIT_SMS_CUTOFF"                // The SMS is blocked after 20:00 IST cutoff
	DecisionExpired             = "WECREDIT_SMS_EXPIRED"               // The SMS is expired and will not be retried
	DecisionCampaignDateInvalid = "WECREDIT_SMS_CAMPAIGN_DATE_INVALID" // The SMS is blocked because its campaign date is missing, malformed, or later than the current business date
)

const (
	CutoffMessage              = "WeCredit SMS blocked after 20:00 IST cutoff; same-day campaign expired and will not be retried."
	ExpiredMessage             = "WeCredit SMS campaign date has expired; message was not sent and will not be retried."
	InvalidCampaignDateMessage = "WeCredit SMS blocked because its campaign date is missing, malformed, or later than the current business date."
)

type Decision struct {
	Code         string
	Message      string
	CampaignDate string
	CurrentIST   time.Time
}

func (d Decision) Allowed() bool { return d.Code == DecisionAllowed }

func (d Decision) ErrorMessage() string {
	if d.Allowed() {
		return ""
	}
	return fmt.Sprintf("[%s] %s", d.Code, d.Message)
}

var (
	istLocation = mustLoadIST()
	clockMu     sync.RWMutex
	nowFunc     = time.Now
)

func mustLoadIST() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		panic(fmt.Sprintf("load Asia/Kolkata: %v", err))
	}
	return loc
}

func Now() time.Time {
	clockMu.RLock()
	fn := nowFunc
	clockMu.RUnlock()
	return fn()
}

// SetClockForTest replaces the policy clock and returns a restore function.
// Tests using it must not run in parallel.
func SetClockForTest(fn func() time.Time) func() {
	clockMu.Lock()
	previous := nowFunc
	if fn == nil {
		nowFunc = time.Now
	} else {
		nowFunc = fn
	}
	clockMu.Unlock()
	return func() {
		clockMu.Lock()
		nowFunc = previous
		clockMu.Unlock()
	}
}

func IST(timeValue time.Time) time.Time { return timeValue.In(istLocation) } // Convert the given time to IST timezone

// IsRegulatedMarketingSMS checks if the given source, sourceRowID, and channel are for a regulated marketing SMS
func IsRegulatedMarketingSMS(source string, sourceRowID int64, channel string) bool {
	return strings.EqualFold(strings.TrimSpace(source), "marketing") &&
		sourceRowID != 0 && strings.EqualFold(strings.TrimSpace(channel), "SMS")
}

// Function to evaluate the decision for a regulated marketing SMS
func Evaluate(source string, sourceRowID int64, channel, campaignDate string, now time.Time) Decision {
	currentIST := IST(now)
	if !IsRegulatedMarketingSMS(source, sourceRowID, channel) {
		return Decision{Code: DecisionAllowed, CampaignDate: campaignDate, CurrentIST: currentIST}
	}

	parsed, err := time.ParseInLocation(CampaignDateLayout, strings.TrimSpace(campaignDate), istLocation)
	if err != nil {
		return Decision{Code: DecisionCampaignDateInvalid, Message: InvalidCampaignDateMessage, CampaignDate: campaignDate, CurrentIST: currentIST}
	}

	currentDate := currentIST.Format(CampaignDateLayout)
	parsedDate := parsed.Format(CampaignDateLayout)

	switch {
	case parsedDate < currentDate:
		return Decision{Code: DecisionExpired, Message: ExpiredMessage, CampaignDate: parsedDate, CurrentIST: currentIST}

	case parsedDate > currentDate:
		return Decision{Code: DecisionCampaignDateInvalid, Message: InvalidCampaignDateMessage, CampaignDate: parsedDate, CurrentIST: currentIST}

	case currentIST.Hour() >= CutoffHour:
		return Decision{Code: DecisionCutoff, Message: CutoffMessage, CampaignDate: parsedDate, CurrentIST: currentIST}

	default:
		return Decision{Code: DecisionAllowed, CampaignDate: parsedDate, CurrentIST: currentIST}
	}
}
