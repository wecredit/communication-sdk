package persistence

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	commredis "github.com/wecredit/communication-sdk/internal/redis"
	"gorm.io/gorm"
)

const (
	AuditSideInput  = commredis.AuditSideInput
	AuditSideOutput = commredis.AuditSideOutput
)

const sqlStatementTimeout = 30 * time.Second

const (
	smsBatchLimit           = 140
	whatsappBatchLimit      = 70
	zapCashCoreBatchLimit   = 50
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
		if IsZapCashCoreMapper(records[0].Mapper) {
			return fmt.Errorf("ZapCash Core database is unavailable")
		}
		return fmt.Errorf("Marketing database is unavailable")
	}
	deduped := dedupeRecords(records)
	switch deduped[0].Mapper {
	case MapperMarketingSMS:
		return writeMarketingSMS(ctx, db, deduped)
	case MapperMarketingWhatsapp:
		return writeMarketingWhatsapp(ctx, db, deduped)
	case MapperZapCashRcsCore, MapperZapCashWhatsappCore, MapperZapCashIvrCore:
		return writeZapCashCore(ctx, db, deduped)
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
		if IsZapCashCoreMapper(record.Mapper) {
			key = record.Mapper + "\x00" + record.CommID
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, record)
	}
	return result
}

func writeZapCashCore(ctx context.Context, db *gorm.DB, records []Record) error {
	if len(records) > zapCashCoreBatchLimit {
		return fmt.Errorf("ZapCash Core persistence batch exceeds limit %d", zapCashCoreBatchLimit)
	}
	rowsByTable := make(map[string][]map[string]interface{})
	marksByTable := make(map[string][]auditMark)
	for _, record := range records {
		if !IsZapCashCoreMapper(record.Mapper) || record.CommID == "" || record.HitAt.IsZero() {
			return fmt.Errorf("invalid ZapCash Core persistence record")
		}
		inputTable, outputTable, err := zapCashCoreTables(record.Mapper)
		if err != nil {
			return err
		}
		input, err := unmarshalAuditMap(record.InputJSON)
		if err != nil {
			return fmt.Errorf("decode input audit: %w", err)
		}
		output, err := unmarshalAuditMap(record.OutputJSON)
		if err != nil {
			return fmt.Errorf("decode output audit: %w", err)
		}
		for _, side := range []struct {
			table, name string
			row         map[string]interface{}
		}{
			{inputTable, AuditSideInput, input}, {outputTable, AuditSideOutput, output},
		} {
			if side.row == nil {
				continue
			}
			field := strings.TrimSpace(record.EventID)
			if field != "" {
				written, err := commredis.IsAuditSideWritten(commredis.RDB, config.Configs.CommIdempotentKey, field, side.name)
				if err == nil && written {
					continue
				}
			}

			side.row["CommId"] = record.CommID
			rowsByTable[side.table] = append(rowsByTable[side.table], side.row)
			marksByTable[side.table] = append(marksByTable[side.table], auditMark{field: field, side: side.name})
		}
	}

	for table, rows := range rowsByTable {
		if err := insertCoreAuditBatch(ctx, db, table, rows); err != nil {
			return err
		}
		for _, mark := range marksByTable[table] {
			if mark.field != "" {
				_ = commredis.MarkAuditSideWritten(commredis.RDB, config.Configs.CommIdempotentKey, mark.field, mark.side)
			}
		}
	}
	return nil
}

type auditMark struct{ field, side string }

func zapCashCoreTables(mapper string) (inputTable, outputTable string, err error) {
	switch mapper {
	case MapperZapCashRcsCore:
		inputTable, err = approvedTable(config.Configs.SdkRcsInputTable)
		if err != nil {
			return "", "", err
		}

		outputTable, err = approvedTable(config.Configs.RcsOutputTable)
		return inputTable, outputTable, err
	case MapperZapCashWhatsappCore:
		inputTable, err = approvedTable(config.Configs.SdkWhatsappInputTable)
		if err != nil {
			return "", "", err
		}
		outputTable, err = approvedTable(config.Configs.WhatsappOutputTable)
		return inputTable, outputTable, err
	case MapperZapCashIvrCore:
		inputTable, err = approvedTable(config.Configs.SdkIvrInputTable)
		if err != nil {
			return "", "", err
		}
		outputTable, err = approvedTable(config.Configs.IvrOutputTable)
		return inputTable, outputTable, err
	default:
		return "", "", fmt.Errorf("unsupported zapcash core mapper %q", mapper)
	}
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

// Core rows are inserted directly (without a per-row transaction) in one
// multi-VALUES statement per audit table. The async stream record remains
// pending if the statement fails, so the existing bounded retry/reclaim path applies.
func insertCoreAuditBatch(parent context.Context, db *gorm.DB, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	columns := make([]string, 0, len(rows[0]))
	for column := range rows[0] {
		columns = append(columns, column)
	}

	sort.Strings(columns)
	values := make([]string, 0, len(rows))
	args := make([]interface{}, 0, len(rows)*len(columns))
	for _, row := range rows {
		if len(row) != len(columns) {
			return fmt.Errorf("inconsistent ZapCash Core audit columns")
		}

		placeholders := make([]string, len(columns))
		for i, column := range columns {
			value, ok := row[column]
			if !ok {
				return fmt.Errorf("inconsistent ZapCash Core audit columns")
			}
			placeholders[i] = "?"
			args = append(args, value)
		}

		values = append(values, "("+strings.Join(placeholders, ",")+")")
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, strings.Join(columns, ","), strings.Join(values, ","))
	ctx, cancel := context.WithTimeout(parent, sqlStatementTimeout)
	defer cancel()
	return db.WithContext(ctx).Exec(query, args...).Error
}

// BatchLimit exposes the stable per-mapper read/write ceiling for startup and
// black-box tests. It does not alter the Redis record contract.
func BatchLimit(mapper string) int64 {
	switch mapper {
	case MapperMarketingSMS:
		return smsBatchLimit
	case MapperZapCashRcsCore, MapperZapCashWhatsappCore, MapperZapCashIvrCore:
		return zapCashCoreBatchLimit
	default:
		return whatsappBatchLimit
	}
}

func BatchParameterCount(mapper string) int {
	switch mapper {
	case MapperMarketingSMS:
		return smsBatchLimit * smsColumnCount
	case MapperZapCashRcsCore, MapperZapCashWhatsappCore, MapperZapCashIvrCore:
		return zapCashCoreBatchLimit
	default:
		return whatsappBatchLimit * whatsappColumnCount
	}
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
