package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/metrics"
	redisstore "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func processAcceptedResult(cfg RuntimeConfig, result AcceptedResult) {
	for _, recipient := range cfg.Recipients {
		processRecipient(cfg, result, recipient)
	}
}

func processRecipient(cfg RuntimeConfig, result AcceptedResult, recipient string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	now := time.Now()
	key := VariantKey(now, result, recipient)
	claimed, err := reserve(ctx, redisstore.RDB, key, now)
	if err != nil {
		monitoringFailure("zapcash_monitor_redis_failure_total", result, err)
		return
	}

	if !claimed {
		metrics.Count("zapcash_monitor_duplicate_skipped_total", result.ResolvedVendor, "zapcash", 1)
		return
	}

	succeeded := false
	defer func() {
		if !succeeded {
			if err := release(context.Background(), redisstore.RDB, key); err != nil {
				monitoringFailure("zapcash_monitor_reservation_release_failure_total", result, err)
			}
		}
	}()

	payload, err := BuildMonitoringPayload(cfg.Profile, result, recipient, key)
	if err != nil {
		monitoringFailure("zapcash_monitor_render_failure_total", result, err)
		return
	}

	if queue.SNSClient == nil {
		monitoringFailure("zapcash_monitor_publish_failure_total", result, fmt.Errorf("SNS client is not initialized"))
		return
	}

	utils.Info(fmt.Sprintf("ZapCash monitoring sending to recipient_tail=%s channel=%s vendor=%s template=%s commId=%s",
		maskMobile(strings.TrimSpace(recipient)), payload.Channel, payload.Vendor, payload.TemplateReference, payload.CommId))
	if err := queue.SendMessageToAwsQueue(queue.SNSClient, payload, config.Configs.AwsSnsArn, variables.Priority); err != nil {
		monitoringFailure("zapcash_monitor_publish_failure_total", result, err)
		return
	}

	succeeded = true
	metrics.Count("zapcash_monitor_enqueued_total", result.ResolvedVendor, "zapcash", 1)
	utils.Info(fmt.Sprintf("ZapCash monitoring sent successfully to recipient_tail=%s channel=%s vendor=%s template=%s",
		maskMobile(strings.TrimSpace(recipient)), payload.Channel, payload.Vendor, payload.TemplateReference))
}

func BuildMonitoringPayload(profile Profile, result AcceptedResult, recipient, key string) (sdkModels.CommApiRequestBody, error) {
	if strings.TrimSpace(result.ResolvedVendor) == "" || strings.TrimSpace(result.ResolvedTemplate) == "" {
		return sdkModels.CommApiRequestBody{}, fmt.Errorf("resolved vendor and template are required")
	}

	if err := validateProfile(profile, result.TemplateVariables+","+result.SMSFallbackVariables); err != nil {
		return sdkModels.CommApiRequestBody{}, err
	}

	payload := result.Payload
	payload.DbClient = nil
	payload.InputTableName = inputTableForChannel(payload.Channel)
	payload.CommId = fmt.Sprintf("ZAPCASH-MON-%s-%d", uuid.New().String(), time.Now().UnixNano())
	payload.EventId = ""
	payload.Source = ""
	payload.SourceRowId = 0
	payload.CampaignDate = ""
	payload.Mobile = recipient
	payload.Email = ""
	payload.Client = "zapcash"
	payload.ProcessName = "ZAPCASH"
	payload.Vendor = strings.ToUpper(strings.TrimSpace(result.ResolvedVendor))
	payload.TemplateReference = strings.TrimSpace(result.ResolvedTemplate)
	payload.IsPriority = true
	payload.IsMonitorCopy = true
	payload.MonitorVariantId = variantID(key)
	payload.CustomerName = profile.CustomerName
	payload.EmiAmount = profile.EmiAmount
	payload.LoanId = profile.LoanID
	payload.ApplicationNumber = profile.ApplicationNumber
	payload.DueDate = profile.DueDate
	payload.Description = profile.Description
	payload.PaymentLink = profile.PaymentLink
	payload.TotalPayableAmount = profile.TotalPayableAmount
	payload.TodayPayableAmount = profile.TodayPayableAmount
	payload.SavingAmount = profile.SavingAmount
	payload.BounceCharge = profile.BounceCharge
	return payload, nil
}

func inputTableForChannel(channel string) string {
	switch strings.ToUpper(strings.TrimSpace(channel)) {
	case "SMS":
		return config.Configs.SdkSmsInputTable
	case "RCS":
		return config.Configs.SdkRcsInputTable
	case "WHATSAPP":
		return config.Configs.SdkWhatsappInputTable
	default:
		return ""
	}
}

func validateProfile(profile Profile, variablesCSV string) error {
	for _, raw := range strings.Split(variablesCSV, ",") {
		variable := strings.TrimSpace(raw)
		key := strings.ToLower(variable)
		if key == "" || key == "zapcashapp" {
			continue
		}
		var value string
		switch key {
		case "customername", "customer_name":
			if variable != "CustomerName" && variable != "Customer_Name" {
				return fmt.Errorf("monitoring profile does not support template variable %q", raw)
			}
			value = profile.CustomerName
		case "overduedays":
			value = profile.DueDate
		case "emiamount":
			value = profile.EmiAmount
		case "loanid":
			value = profile.LoanID
		case "applicationnumber":
			value = profile.ApplicationNumber
		case "duedate":
			value = profile.DueDate
		case "description":
			value = profile.Description
		case "paymentlink", "link":
			value = profile.PaymentLink
		case "totalpayableamount":
			value = profile.TotalPayableAmount
		case "todaypayableamount":
			value = profile.TodayPayableAmount
		case "savingamount":
			value = profile.SavingAmount
		case "bouncecharge":
			value = profile.BounceCharge
		default:
			return fmt.Errorf("monitoring profile does not support template variable %q", raw)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("monitoring profile value is missing for template variable %q", raw)
		}
	}
	return nil
}

func variantID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}

func monitoringFailure(metric string, result AcceptedResult, err error) {
	metrics.Count(metric, result.ResolvedVendor, "zapcash", 1)
	utils.Error(fmt.Errorf("ZapCash monitoring failure channel=%s stage=%.2f template=%s: %v",
		result.Payload.Channel, result.Payload.Stage, result.ResolvedTemplate, err))
}
