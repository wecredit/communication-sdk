package database

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

// Approved dbo.CommDispatchTracking column sizes (Marketing SQL Server).
const (
	DispatchTrackingSent             = "SENT"
	DispatchTrackingFailed           = "FAILED"
	DispatchTrackingSkippedDuplicate = "SKIPPED_DUPLICATE"

	trackingSourceMax   = 32
	trackingChannelMax  = 20
	trackingClientMax   = 30
	trackingVendorMax   = 30
	trackingProcessMax  = 50
	trackingEventIdMax  = 100
	trackingCommIdMax   = 100
	trackingTxnIdMax    = 100
	trackingErrorMax    = 500
	trackingTemplateMax = 100
)

var (
	ErrDispatchTrackingAlreadyExists = errors.New("dispatch tracking row already exists")
	trackingSensitiveNumber          = regexp.MustCompile(`\b[0-9]{10,15}\b`)
)

type CommDispatchTrackingRow struct {
	Source            string
	SourceRowId       int64
	Channel           string
	Client            string
	Vendor            string
	Process           string
	EventId           string
	CommId            string
	TransactionId     string
	Outcome           string
	ErrorMessage      string
	TemplateReference string
}

func InsertCommDispatchTracking(db *gorm.DB, tableName string, row CommDispatchTrackingRow) error {
	if db == nil {
		return fmt.Errorf("marketing database is not initialized")
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return fmt.Errorf("tracking table name is required")
	}

	source := clampTracking(row.Source, trackingSourceMax)
	channel := clampTracking(row.Channel, trackingChannelMax)
	if source == "" || row.SourceRowId == 0 || channel == "" {
		return fmt.Errorf("source, sourceRowId, and channel are required")
	}
	if row.Outcome != DispatchTrackingSent && row.Outcome != DispatchTrackingFailed && row.Outcome != DispatchTrackingSkippedDuplicate {
		return fmt.Errorf("invalid tracking outcome %q", row.Outcome)
	}

	// Approved table: PK on Id only, DF CreatedOn/UpdatedOn = SYSDATETIME(), AppliedOn left NULL.
	// No unique index — skip a second row for the same (Source, SourceRowId, Channel).
	query := fmt.Sprintf(`INSERT INTO %s (
		[Source], SourceRowId, Channel, Client, Vendor, Process, EventId,
		CommId, TransactionId, Outcome, ErrorMessage, TemplateReference,
		CreatedOn, UpdatedOn
	)
	SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, SYSDATETIME(), SYSDATETIME()
	WHERE NOT EXISTS (
		SELECT 1 FROM %s t
		WHERE t.[Source] = ? AND t.SourceRowId = ? AND t.Channel = ?
	)`, tableName, tableName)

	result := db.Exec(query,
		source,
		row.SourceRowId,
		channel,
		nullIfBlank(clampTracking(row.Client, trackingClientMax)),
		nullIfBlank(clampTracking(row.Vendor, trackingVendorMax)),
		nullIfBlank(clampTracking(row.Process, trackingProcessMax)),
		nullIfBlank(clampTracking(row.EventId, trackingEventIdMax)),
		nullIfBlank(clampTracking(row.CommId, trackingCommIdMax)),
		nullIfBlank(clampTracking(row.TransactionId, trackingTxnIdMax)),
		row.Outcome,
		nullIfBlank(sanitizeTrackingError(row.ErrorMessage)),
		nullIfBlank(clampTracking(row.TemplateReference, trackingTemplateMax)),
		source,
		row.SourceRowId,
		channel,
	)
	if result.Error != nil {
		if isSQLServerUniqueViolation(result.Error) {
			return ErrDispatchTrackingAlreadyExists
		}
		return fmt.Errorf("insert dispatch tracking: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrDispatchTrackingAlreadyExists
	}
	utils.Info(fmt.Sprintf("inserted dispatch tracking sourceRowId=%d outcome=%s", row.SourceRowId, row.Outcome))
	return nil
}

func clampTracking(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func nullIfBlank(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func sanitizeTrackingError(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	message = trackingSensitiveNumber.ReplaceAllString(message, "[REDACTED]")
	return clampTracking(message, trackingErrorMax)
}

func isSQLServerUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	type numbered interface{ SQLErrorNumber() int32 }
	var n numbered
	if errors.As(err, &n) {
		switch n.SQLErrorNumber() {
		case 2627, 2601:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "2627") || strings.Contains(msg, "2601") ||
		strings.Contains(msg, "unique key") || strings.Contains(msg, "duplicate key")
}
