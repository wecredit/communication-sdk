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

const (
	smsBatchLimit           = 140
	whatsappBatchLimit      = 70
	smsColumnCount          = 13
	whatsappColumnCount     = 21
	sqlServerParameterLimit = 2100
)

var safeTableName = regexp.MustCompile(`^[A-Za-z0-9_\.\[\]]+$`)
var marketingLocation = time.FixedZone("IST", 5*60*60+30*60)

func writeRecords(ctx context.Context, db *gorm.DB, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	if db == nil {
		return fmt.Errorf("Marketing database is unavailable")
	}
	deduped := dedupeRecords(records)
	switch deduped[0].Mapper {
	case MapperMarketingSMS:
		return writeMarketingSMS(ctx, db, deduped)
	case MapperMarketingWhatsapp:
		return writeMarketingWhatsapp(ctx, db, deduped)
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

func writeMarketingSMS(ctx context.Context, db *gorm.DB, records []Record) error {
	table, err := approvedTable(config.Configs.CommDispatchTrackingTable)
	if err != nil {
		return err
	}
	if len(records) > smsBatchLimit || len(records)*smsColumnCount > sqlServerParameterLimit {
		return fmt.Errorf("SMS persistence batch exceeds SQL Server parameter limit")
	}
	rows := make([]string, 0, len(records))
	args := make([]interface{}, 0, len(records)*smsColumnCount)
	for _, record := range records {
		if record.Mapper != MapperMarketingSMS || record.SourceRowID < 1 || record.Source == "" || record.HitAt.IsZero() {
			return fmt.Errorf("invalid SMS persistence record")
		}
		rows = append(rows, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, SYSDATETIME(), ?)")
		args = append(args,
			record.Source, record.SourceRowID, "SMS", nullString(record.Client), nullString(record.Vendor),
			nullString(record.Process), nullString(record.EventID), nullString(record.CommID),
			nullString(record.TransactionID), record.Outcome, nullString(record.ErrorMessage),
			nullString(record.TemplateReference), record.HitAt.In(marketingLocation),
		)
	}
	query := fmt.Sprintf(`INSERT INTO %s
([Source], SourceRowId, Channel, Client, Vendor, Process, EventId, CommId, TransactionId, Outcome, ErrorMessage, TemplateReference, CreatedOn, UpdatedOn)
	VALUES %s`, table, strings.Join(rows, ","))
	return executeBatch(ctx, db, query, args)
}

func writeMarketingWhatsapp(ctx context.Context, db *gorm.DB, records []Record) error {
	table, err := approvedTable(config.Configs.CommWhatsappMarketingOutputTable)
	if err != nil {
		return err
	}
	if len(records) > whatsappBatchLimit || len(records)*whatsappColumnCount > sqlServerParameterLimit {
		return fmt.Errorf("WhatsApp persistence batch exceeds SQL Server parameter limit")
	}
	rows := make([]string, 0, len(records))
	args := make([]interface{}, 0, len(records)*whatsappColumnCount)
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
	VALUES %s`, table, strings.Join(rows, ","))
	return executeBatch(ctx, db, query, args)
}

func executeBatch(parent context.Context, db *gorm.DB, query string, args []interface{}) error {
	ctx, cancel := context.WithTimeout(parent, sqlStatementTimeout)
	defer cancel()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Exec(query, args...).Error
	})
}

// BatchLimit exposes the stable per-mapper read/write ceiling for startup and
// black-box tests. It does not alter the Redis record contract.
func BatchLimit(mapper string) int64 {
	if mapper == MapperMarketingSMS {
		return smsBatchLimit
	}
	return whatsappBatchLimit
}

func BatchParameterCount(mapper string) int {
	if mapper == MapperMarketingSMS {
		return smsBatchLimit * smsColumnCount
	}
	return whatsappBatchLimit * whatsappColumnCount
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
