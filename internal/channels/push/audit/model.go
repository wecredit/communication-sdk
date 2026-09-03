package audit

import "time"

// Input records the accepted PUSH request and resolved content once per event.
// Raw device tokens and Firebase credentials must never be stored here.
type Input struct {
	ID                uint64    `gorm:"column:Id;primaryKey"`
	CommID            string    `gorm:"column:CommId"`
	EventID           string    `gorm:"column:EventId"`
	Client            string    `gorm:"column:Client"`
	ProcessName       string    `gorm:"column:ProcessName"`
	Stage             float64   `gorm:"column:Stage"`
	Vendor            string    `gorm:"column:Vendor"`
	TemplateName      string    `gorm:"column:TemplateName"`
	Title             string    `gorm:"column:Title"`
	Body              string    `gorm:"column:Body"`
	NotificationEvent string    `gorm:"column:NotificationEvent"`
	DeepLink          string    `gorm:"column:DeepLink"`
	UserID            string    `gorm:"column:UserId"`
	ApplicationNumber string    `gorm:"column:ApplicationNumber"`
	DeviceCount       int       `gorm:"column:DeviceCount"`
	CreatedOn         time.Time `gorm:"column:CreatedOn"`
}

// Output records one terminal FCM result per token fingerprint.
type Output struct {
	ID                uint64    `gorm:"column:Id;primaryKey"`
	LedgerID          uint64    `gorm:"column:LedgerId"`
	CommID            string    `gorm:"column:CommId"`
	EventID           string    `gorm:"column:EventId"`
	Client            string    `gorm:"column:Client"`
	TokenFingerprint  string    `gorm:"column:TokenFingerprint"`
	Outcome           string    `gorm:"column:Outcome"`
	AttemptCount      int       `gorm:"column:AttemptCount"`
	ErrorCode         string    `gorm:"column:ErrorCode"`
	ProviderMessageID string    `gorm:"column:ProviderMessageId"`
	CreatedOn         time.Time `gorm:"column:CreatedOn"`
}
