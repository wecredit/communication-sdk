package persistence

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"gorm.io/gorm"
)

const sqlStatementTimeout = 30 * time.Second

var safeTableName = regexp.MustCompile(`^[A-Za-z0-9_\.\[\]]+$`)
var marketingLocation = time.FixedZone("IST", 5*60*60+30*60)

func writeRecords(db *gorm.DB, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	if db == nil {
		return fmt.Errorf("Marketing database is unavailable")
	}
	deduped := dedupeRecords(records)
	switch deduped[0].Mapper {
	case MapperMarketingSMS:
		return writeMarketingSMS(db, deduped)
	case MapperMarketingWhatsapp:
		return writeMarketingWhatsapp(db, deduped)
	default:
		return fmt.Errorf("unsupported persistence mapper %q", deduped[0].Mapper)
	}
}

func dedupeRecords(records []Record) []Record {
	seen := make(map[string]struct{}, len(records))
	result := make([]Record, 0, len(records))
	for _, record := range records {
		key := record.Mapper + "\x00" + record.Source + "\x00" + fmt.Sprint(record.SourceRowID) + "\x00" + record.Channel
		if record.Mapper == MapperMarketingWhatsapp {
			key = record.Mapper + "\x00" + fmt.Sprint(record.SourceRowID)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, record)
	}
	return result
}

func writeMarketingSMS(db *gorm.DB, records []Record) error {
	table, err := approvedTable(config.Configs.CommDispatchTrackingTable)
	if err != nil {
		return err
	}
	const columns = 13
	if len(records)*columns > 2100 {
		return fmt.Errorf("SMS persistence batch exceeds SQL Server parameter limit")
	}
	rows := make([]string, 0, len(records))
	args := make([]interface{}, 0, len(records)*columns)
	for _, record := range records {
		if record.Mapper != MapperMarketingSMS || record.SourceRowID < 1 || record.Source == "" || record.HitAt.IsZero() {
			return fmt.Errorf("invalid SMS persistence record")
		}
		rows = append(rows, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			record.Source, record.SourceRowID, "SMS", nullString(record.Client), nullString(record.Vendor),
			nullString(record.Process), nullString(record.EventID), nullString(record.CommID),
			nullString(record.TransactionID), record.Outcome, nullString(record.ErrorMessage),
			nullString(record.TemplateReference), record.HitAt.In(marketingLocation),
		)
	}
	query := fmt.Sprintf(`INSERT INTO %s
([Source], SourceRowId, Channel, Client, Vendor, Process, EventId, CommId, TransactionId, Outcome, ErrorMessage, TemplateReference, CreatedOn, UpdatedOn)
SELECT v.[Source], v.SourceRowId, v.Channel, v.Client, v.Vendor, v.Process, v.EventId, v.CommId, v.TransactionId, v.Outcome, v.ErrorMessage, v.TemplateReference, SYSDATETIME(), v.HitAt
FROM (VALUES %s) AS v([Source], SourceRowId, Channel, Client, Vendor, Process, EventId, CommId, TransactionId, Outcome, ErrorMessage, TemplateReference, HitAt)
WHERE NOT EXISTS (
    SELECT 1 FROM %s AS existing
    WHERE existing.[Source] = v.[Source]
      AND existing.SourceRowId = v.SourceRowId
      AND existing.Channel = v.Channel
)`, table, strings.Join(rows, ","), table)
	return executeBatch(db, query, args)
}

func writeMarketingWhatsapp(db *gorm.DB, records []Record) error {
	table, err := approvedTable(config.Configs.CommWhatsappMarketingOutputTable)
	if err != nil {
		return err
	}
	const columns = 21
	if len(records)*columns > 2100 {
		return fmt.Errorf("WhatsApp persistence batch exceeds SQL Server parameter limit")
	}
	rows := make([]string, 0, len(records))
	args := make([]interface{}, 0, len(records)*columns)
	for _, record := range records {
		if record.Mapper != MapperMarketingWhatsapp || record.SourceRowID < 1 || record.HitAt.IsZero() {
			return fmt.Errorf("invalid WhatsApp persistence record")
		}
		rows = append(rows, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			record.SourceRowID, nullString(record.EventID), nullString(record.CommID), nullString(record.Mobile),
			nullString(record.Vendor), nullString(record.Process), nullString(record.Client), nullString(record.TemplateReference),
			nullString(record.AppID), nullString(record.TransactionID), nullString(record.ProviderMessageID), record.IsSent,
			nullString(record.ErrorMessage), record.HitAt.In(marketingLocation), nullString(record.Tag1), nullString(record.Tag2),
			nullString(record.DynamicMobile), nullString(record.VariablesValue), nullableTime(record.ScheduledAt),
			nullString(record.RawPayload), nullString(record.RawResponse),
		)
	}
	query := fmt.Sprintf(`INSERT INTO %s
(SourceRowId, EventId, CommId, Mobile, Vendor, Process, Client, TemplateName, AppId, TransactionId, MessageId, IsSent, ResponseMessage, HitTime, Tag1, Tag2, DynamicMobile, VariablesValue, ScheduledAt, RawPayload, RawResponse)
SELECT v.SourceRowId, v.EventId, v.CommId, v.Mobile, v.Vendor, v.Process, v.Client, v.TemplateName, v.AppId, v.TransactionId, v.MessageId, v.IsSent, v.ResponseMessage, v.HitTime, v.Tag1, v.Tag2, v.DynamicMobile, v.VariablesValue, v.ScheduledAt, v.RawPayload, v.RawResponse
FROM (VALUES %s) AS v(SourceRowId, EventId, CommId, Mobile, Vendor, Process, Client, TemplateName, AppId, TransactionId, MessageId, IsSent, ResponseMessage, HitTime, Tag1, Tag2, DynamicMobile, VariablesValue, ScheduledAt, RawPayload, RawResponse)
WHERE NOT EXISTS (
    SELECT 1 FROM %s AS existing WHERE existing.SourceRowId = v.SourceRowId
)`, table, strings.Join(rows, ","), table)
	return executeBatch(db, query, args)
}

func executeBatch(db *gorm.DB, query string, args []interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), sqlStatementTimeout)
	defer cancel()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Exec(query, args...).Error
	})
}

func approvedTable(table string) (string, error) {
	table = strings.TrimSpace(table)
	if table == "" || !safeTableName.MatchString(table) {
		return "", fmt.Errorf("invalid configured persistence table %q", table)
	}
	return table, nil
}

func nullString(value string) interface{} {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return nil
}

func nullableTime(value *time.Time) interface{} {
	if value == nil || value.IsZero() {
		return nil
	}
	return *value
}
