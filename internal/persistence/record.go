package persistence

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	redislib "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/models/redisModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

const (
	MapperMarketingSMS      = "marketing_sms_tracking_v1"
	MapperMarketingWhatsapp = "marketing_whatsapp_output_v1"
)

var ErrMissingHitAt = errors.New("terminal result is missing HitAt")

// Record is the fixed Redis Streams persistence contract. It contains values,
// never table names or executable SQL.
type Record struct {
	Mapper            string
	EventID           string
	Source            string
	SourceRowID       int64
	Channel           string
	Client            string
	Vendor            string
	Process           string
	CommID            string
	TransactionID     string
	ProviderMessageID string
	Outcome           string
	ErrorMessage      string
	TemplateReference string
	HitAt             time.Time
	Mobile            string
	AppID             string
	IsSent            bool
	Tag1              string
	Tag2              string
	DynamicMobile     string
	VariablesValue    string
	ScheduledAt       *time.Time
	RawPayload        string
	RawResponse       string
}

func MarketingSMSRecord(data sdkModels.CommApiRequestBody, output map[string]interface{}, terminal redisModels.MobileChannelRedisData) (Record, error) {
	hitAt, err := terminalHitAt(terminal)
	if err != nil {
		return Record{}, err
	}

	transactionID := firstNonBlank(terminal.ProviderMessageID, terminal.TransactionId, mapString(output, "TransactionId"))
	outcome := strings.ToUpper(strings.TrimSpace(terminal.Outcome))
	if outcome == "" {
		outcome = strings.ToUpper(mapString(output, "Outcome"))
	}

	switch outcome {
	case "SUBMITTED", "ALREADY_DONE", "SENT":
		outcome = "SENT"
	case "FAILED_FINAL", "SKIPPED", "REJECTED", "FAILED", "":
		outcome = "FAILED"
	}

	normalized, err := database.NormalizeCommDispatchTrackingRow(database.CommDispatchTrackingRow{
		Source: strings.TrimSpace(data.Source), SourceRowId: data.SourceRowId, Channel: "SMS",
		Client: strings.ToLower(strings.TrimSpace(data.Client)), Vendor: strings.TrimSpace(data.Vendor),
		Process: strings.TrimSpace(data.ProcessName), EventId: strings.TrimSpace(data.EventId),
		CommId: strings.TrimSpace(data.CommId), TransactionId: transactionID, Outcome: outcome,
		ErrorMessage:      firstNonBlank(terminal.ErrorMessage, mapString(output, "ResponseMessage")),
		TemplateReference: strings.TrimSpace(data.TemplateReference),
	})
	if err != nil {
		return Record{}, err
	}

	return Record{
		Mapper:            MapperMarketingSMS,
		EventID:           normalized.EventId,
		Source:            normalized.Source,
		SourceRowID:       normalized.SourceRowId,
		Channel:           normalized.Channel,
		Client:            normalized.Client,
		Vendor:            normalized.Vendor,
		Process:           normalized.Process,
		CommID:            normalized.CommId,
		TransactionID:     normalized.TransactionId,
		ProviderMessageID: firstNonBlank(terminal.ProviderMessageID, transactionID),
		Outcome:           normalized.Outcome,
		ErrorMessage:      normalized.ErrorMessage,
		TemplateReference: normalized.TemplateReference,
		HitAt:             hitAt,
	}, nil
}

func MarketingWhatsappRecord(data sdkModels.CommApiRequestBody, output map[string]interface{}, terminal redisModels.MobileChannelRedisData) (Record, error) {
	hitAt, err := terminalHitAt(terminal)
	if err != nil {
		return Record{}, err
	}

	transactionID := firstNonBlank(terminal.ProviderMessageID, terminal.TransactionId, mapString(output, "TransactionId"))
	isSent := mapBool(output, "IsSent") || strings.EqualFold(terminal.Outcome, "submitted")
	record := Record{
		Mapper:            MapperMarketingWhatsapp,
		EventID:           strings.TrimSpace(data.EventId),
		SourceRowID:       data.SourceRowId,
		Channel:           "WHATSAPP",
		Client:            strings.ToLower(strings.TrimSpace(data.Client)),
		Vendor:            strings.TrimSpace(data.Vendor),
		Process:           strings.TrimSpace(data.ProcessName),
		CommID:            strings.TrimSpace(data.CommId),
		TransactionID:     transactionID,
		ProviderMessageID: firstNonBlank(terminal.ProviderMessageID, transactionID),
		Outcome:           strings.TrimSpace(terminal.Outcome),
		ErrorMessage:      firstNonBlank(terminal.ErrorMessage, mapString(output, "ResponseMessage")),
		TemplateReference: strings.TrimSpace(data.TemplateReference),
		HitAt:             hitAt,
		Mobile:            strings.TrimSpace(data.Mobile),
		AppID:             firstNonBlank(mapString(output, "AppId"), strings.TrimSpace(data.AppId)),
		IsSent:            isSent,
		Tag1:              strings.TrimSpace(data.Tag1),
		Tag2:              strings.TrimSpace(data.Tag2),
		DynamicMobile:     strings.TrimSpace(data.DynamicMobile),
		VariablesValue:    strings.TrimSpace(data.TemplateVariableValues),
		RawPayload:        mapRawString(output, "RawPayload"),
		RawResponse:       mapRawString(output, "RawResponse"),
	}

	if !data.ScheduledAt.IsZero() {
		scheduled := data.ScheduledAt
		record.ScheduledAt = &scheduled
	}

	return record, nil
}

func terminalHitAt(terminal redisModels.MobileChannelRedisData) (time.Time, error) {
	raw := strings.TrimSpace(terminal.HitAt)
	if raw == "" {
		return time.Time{}, ErrMissingHitAt
	}

	hitAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid terminal HitAt: %w", err)
	}
	return hitAt, nil
}

func (r Record) streamValues() map[string]interface{} {
	values := map[string]interface{}{
		"mapper": r.Mapper, "event_id": r.EventID, "source": r.Source,
		"source_row_id": strconv.FormatInt(r.SourceRowID, 10), "channel": r.Channel,
		"client": r.Client, "vendor": r.Vendor, "process": r.Process,
		"comm_id": r.CommID, "transaction_id": r.TransactionID,
		"provider_message_id": r.ProviderMessageID, "outcome": r.Outcome,
		"error_message": r.ErrorMessage, "template_reference": r.TemplateReference,
		"hit_at": r.HitAt.Format(time.RFC3339Nano), "mobile": r.Mobile,
		"app_id": r.AppID, "is_sent": strconv.FormatBool(r.IsSent), "tag1": r.Tag1,
		"tag2": r.Tag2, "dynamic_mobile": r.DynamicMobile,
		"variables_value": r.VariablesValue, "raw_payload": r.RawPayload,
		"raw_response": r.RawResponse,
	}
	if r.ScheduledAt != nil {
		values["scheduled_at"] = r.ScheduledAt.Format(time.RFC3339Nano)
	} else {
		values["scheduled_at"] = ""
	}
	return values
}

func recordFromMessage(message redislib.XMessage) (Record, error) {
	value := func(key string) string { return strings.TrimSpace(fmt.Sprint(message.Values[key])) }
	rawValue := func(key string) string { return fmt.Sprint(message.Values[key]) }
	sourceRowID, err := strconv.ParseInt(value("source_row_id"), 10, 64)
	if err != nil || sourceRowID < 1 {
		return Record{}, fmt.Errorf("invalid source_row_id %q", value("source_row_id"))
	}

	hitAt, err := time.Parse(time.RFC3339Nano, value("hit_at"))
	if err != nil {
		return Record{}, fmt.Errorf("invalid hit_at: %w", err)
	}

	isSent, err := strconv.ParseBool(value("is_sent"))
	if err != nil {
		return Record{}, fmt.Errorf("invalid is_sent: %w", err)
	}

	record := Record{
		Mapper: value("mapper"), EventID: value("event_id"), Source: value("source"),
		SourceRowID: sourceRowID, Channel: value("channel"), Client: value("client"),
		Vendor: value("vendor"), Process: value("process"), CommID: value("comm_id"),
		TransactionID: value("transaction_id"), ProviderMessageID: value("provider_message_id"),
		Outcome: value("outcome"), ErrorMessage: value("error_message"),
		TemplateReference: value("template_reference"), HitAt: hitAt, Mobile: value("mobile"),
		AppID: value("app_id"), IsSent: isSent, Tag1: value("tag1"), Tag2: value("tag2"),
		DynamicMobile: value("dynamic_mobile"), VariablesValue: value("variables_value"),
		RawPayload: rawValue("raw_payload"), RawResponse: rawValue("raw_response"),
	}

	if raw := value("scheduled_at"); raw != "" {
		scheduled, parseErr := time.Parse(time.RFC3339Nano, raw)
		if parseErr != nil {
			return Record{}, fmt.Errorf("invalid scheduled_at: %w", parseErr)
		}
		record.ScheduledAt = &scheduled
	}

	if record.Mapper != MapperMarketingSMS && record.Mapper != MapperMarketingWhatsapp {
		return Record{}, fmt.Errorf("unsupported mapper %q", record.Mapper)
	}

	return record, nil
}

func mapString(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}

	value, exists := values[key]
	if !exists || value == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(value))
}

func mapRawString(values map[string]interface{}, key string) string {
	if values == nil || values[key] == nil {
		return ""
	}
	return fmt.Sprint(values[key])
}

func mapBool(values map[string]interface{}, key string) bool {
	switch value := values[key].(type) {
	case bool:
		return value
	case int:
		return value == 1
	case int64:
		return value == 1
	case float64:
		return value == 1
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
		return parsed || strings.TrimSpace(value) == "1"
	default:
		return false
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
