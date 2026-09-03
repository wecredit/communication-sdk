package ledger

import "time"

type Status string

const (
	StatusPending        Status = "pending"
	StatusClaimed        Status = "claimed"
	StatusRetryable      Status = "retryable"
	StatusSubmitted      Status = "submitted"
	StatusFailedFinal    Status = "failed_final"
	StatusCancelledStale Status = "cancelled_stale"

	ClaimExpiry     = 5 * time.Minute
	ClaimAgeWarning = 3 * time.Minute
)

// Dispatch is the durable, token-level PUSH delivery state. Device tokens are
// intentionally excluded; TokenFingerprint is a non-reversible SHA-256 value.
type Dispatch struct {
	ID                  uint64     `gorm:"column:Id;primaryKey"`
	Client              string     `gorm:"column:Client"`
	EventID             string     `gorm:"column:EventId"`
	Campaign            string     `gorm:"column:Campaign"`
	CampaignDate        string     `gorm:"column:CampaignDate"`
	Variant             string     `gorm:"column:Variant"`
	EligibilityIdentity string     `gorm:"column:EligibilityIdentity"`
	TokenFingerprint    string     `gorm:"column:TokenFingerprint"`
	Status              Status     `gorm:"column:Status"`
	AttemptCount        int        `gorm:"column:AttemptCount"`
	ReclaimCount        int        `gorm:"column:ReclaimCount"`
	ErrorCode           string     `gorm:"column:ErrorCode"`
	ProviderMessageID   string     `gorm:"column:ProviderMessageId"`
	ClaimedAt           *time.Time `gorm:"column:ClaimedAt"`
	LastAttemptAt       *time.Time `gorm:"column:LastAttemptAt"`
	FinalizedAt         *time.Time `gorm:"column:FinalizedAt"`
	CreatedOn           time.Time  `gorm:"column:CreatedOn"`
	UpdatedOn           time.Time  `gorm:"column:UpdatedOn"`
}

func (Dispatch) TableName() string {
	return "PushDispatchLedger"
}

func (s Status) Terminal() bool {
	switch s {
	case StatusSubmitted, StatusFailedFinal, StatusCancelledStale:
		return true
	default:
		return false
	}
}
