// Query-campaign preview/materialize helpers live under sdk/helper (shared layer),
// not a feature package. Used by admin APIs and nurture materialize.
package sdkHelper

import (
	"context"
	"database/sql"
	"time"
)

const (
	SourceQueryCampaign = "query_campaign"

	StatusDraft    = "draft"
	StatusApproved = "approved"
	StatusEnabled  = "enabled"
	StatusPaused   = "paused"

	PreviewRunning = "running"
	PreviewDone    = "done"
	PreviewFailed  = "failed"

	RunRunning = "running"
	RunSuccess = "success"
	RunFailed  = "failed"
	RunSkipped = "skipped"

	LedgerPending    = "PENDING"
	LedgerProcessing = "PROCESSING"
	LedgerSending    = "SENDING"
	LedgerSent       = "SENT"
	LedgerFailed     = "FAILED"
	LedgerExpired    = "EXPIRED"
	LedgerNotHit     = "NOT_HIT"

	ConsumedEligible = "0"
	ConsumedInFlight = "2"
	ConsumedTerminal = "1"

	ChannelSMS      = "SMS"
	ChannelWhatsApp = "WHATSAPP"

	ClientZapCash  = "zapcash"
	ClientWeCredit = "wecredit"

	DbTargetMarketingSQLServer = "marketing-sqlserver"
	DbTargetZapCashMySQLV1     = "zapcash-mysql-v1"
	DbTargetCommunicationMySQL = "communication-mysql"

	DefaultMaterializeHardCap        = 250000
	DefaultExpectedRowsMarginPercent = 20
	DefaultPreviewMaxRows            = 100
	DefaultPreviewMaxAgeHours        = 24
	DefaultPreviewStaleBufferSeconds = 60
	DefaultSendingGraceSeconds       = 900
	DefaultMaxQuerySeconds           = 900
	DefaultInsertBudgetSeconds       = 300
)

// Schedule is the campaign ScheduleJson shape.
type Schedule struct {
	Timezone   string `json:"timezone"`
	Time       string `json:"time"` // HH:MM IST
	Frequency  string `json:"frequency"`
	OnceDate   string `json:"onceDate,omitempty"`
	DaysOfWeek []int  `json:"daysOfWeek,omitempty"`
	DayOfMonth int    `json:"dayOfMonth,omitempty"`
}

// CampaignRow is one validated SELECT result row ready for ledger insert.
// Template placeholders use VariableValues (positional CSV) only.
type CampaignRow struct {
	Client            string
	Channel           string
	Vendor            string
	ProcessName       string
	Mobile            string
	TemplateReference string
	AppId             string
	VariableValues    string
}

// PreviewResult is produced by Mode preview (no ledger writes).
// SampleRows is how many rows were validated (capped at DefaultPreviewMaxRows).
type PreviewResult struct {
	SampleRows int
	Duration   time.Duration
	Error      string
}

// MaterializeSink receives validated rows in batches inside one transaction.
type MaterializeSink interface {
	InsertBatch(ctx context.Context, rows []CampaignRow) error
}

// MaterializeResult summarizes a successful stream (caller commits).
type MaterializeResult struct {
	RowsSelected       int
	RowsInserted       int
	RowsSkippedInvalid int
	Duration           time.Duration
}

// TemplateChecker reports whether templateReference exists and is approved.
type TemplateChecker interface {
	TemplateApproved(ctx context.Context, client, channel, process, templateReference string) (bool, error)
}

// RunConfig configures a preview or materialize execution.
type RunConfig struct {
	SqlText            string
	DbTargetID         string
	Client             string // campaign Client: zapcash | wecredit
	Channel            string
	QueryTimeoutSec    int
	MaxQuerySeconds    int
	PreviewMaxRows     int // preview only; 0 → DefaultPreviewMaxRows
	MaterializeHardCap int
	AbortThreshold     int // ApprovedExpectedRows * (1+margin); 0 = hard cap only
	DB                 *sql.DB
	Templates          TemplateChecker
}

// EffectiveTimeout returns min(QueryTimeoutSec, MaxQuerySeconds) with sane floors.
func (c RunConfig) EffectiveTimeout() time.Duration {
	maxSec := c.MaxQuerySeconds
	if maxSec <= 0 {
		maxSec = DefaultMaxQuerySeconds
	}
	sec := c.QueryTimeoutSec
	if sec <= 0 {
		sec = maxSec
	}
	if sec > maxSec {
		sec = maxSec
	}
	return time.Duration(sec) * time.Second
}
