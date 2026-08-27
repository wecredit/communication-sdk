package sms

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	pinnacleSms "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle"
	sinchSms "github.com/wecredit/communication-sdk/internal/channels/sms/sinch"
	"github.com/wecredit/communication-sdk/internal/channels/sms/templatevars"
	timesSms "github.com/wecredit/communication-sdk/internal/channels/sms/times"
	"github.com/wecredit/communication-sdk/internal/metrics"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	smspolicy "github.com/wecredit/communication-sdk/sdk/policy"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// SendSmsResult is the consumer-facing result of an SMS provider attempt.
type SendSmsResult struct {
	Processed bool
	AckSQS    bool
	DBData    map[string]interface{}
}

// SendSmsByProcess resolves template/vendor, calls the provider, and classifies the outcome.
// Stage/day frequency idempotency remains Redis on the SDK Send path.
// Marketing rows use EventId (marketing-<Id>) as the Redis field so same-mobile
// stage-0 / template-reference sends do not collide. CommId stays WC-<CLIENT>-...
// Durable CommId provider-claim is deferred until a store is approved.
func SendSmsByProcess(msg sdkModels.CommApiRequestBody) (SendSmsResult, error) {
	req := smsRequestFromMessage(msg)
	if decision := smspolicy.Evaluate(msg.Source, msg.SourceRowId, msg.Channel, msg.CampaignDate, smspolicy.Now()); !decision.Allowed() {
		return complianceBlockedResult(msg, req, decision)
	}

	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		return SendSmsResult{}, errors.New("template data not found in cache")
	}
	templateData, matchedVendor, err := channelHelper.ResolveTemplateData(msg, templateDetails)
	if err != nil {
		ok, dbData, handleErr := channelHelper.HandleTemplateNotFoundError(msg, err)
		return SendSmsResult{Processed: ok, AckSQS: ok, DBData: dbData}, handleErr
	}

	msg.Vendor = matchedVendor

	req = extapimodels.SmsRequestBody{
		Mobile:                 msg.Mobile,
		Process:                msg.ProcessName,
		Client:                 msg.Client,
		Channel:                msg.Channel,
		CommId:                 msg.CommId,
		Vendor:                 msg.Vendor,
		EmiAmount:              msg.EmiAmount,
		CustomerName:           msg.CustomerName,
		LoanId:                 msg.LoanId,
		ApplicationNumber:      msg.ApplicationNumber,
		DueDate:                msg.DueDate,
		Description:            msg.Description,
		PaymentLink:            msg.PaymentLink,
		TemplateVariableValues: msg.TemplateVariableValues,
		Source:                 msg.Source,
		SourceRowId:            msg.SourceRowId,
		CampaignDate:           msg.CampaignDate,
	}
	channelHelper.PopulateSmsFields(&req, templateData)

	if strings.Contains(req.TemplateText, "{#var#}") {
		resolvedText, applyErr := templatevars.ApplyTemplateVariables(req)
		if applyErr != nil {
			utils.Error(fmt.Errorf("SMS template variable substitution failed for CommId %s: %v", msg.CommId, applyErr))
			response := extapimodels.SmsResponse{
				Outcome:         outcome.FailedFinal,
				ResponseMessage: fmt.Sprintf("template variable substitution failed: %v", applyErr),
			}
			return buildSmsResult(msg, req, response)
		}
		req.TemplateText = resolvedText
	}

	var response extapimodels.SmsResponse
	shouldHitVendor := channelHelper.ShouldHitVendor(msg.Client, msg.Channel)
	utils.Debug(fmt.Sprintf("Channel: %s CommId: %s, Should hit vendor: %v\n", msg.Channel, msg.CommId, shouldHitVendor))

	if !shouldHitVendor {
		response.Outcome = outcome.Skipped
		response.ResponseMessage = "shouldHitVendor is off for mobile " + msg.Mobile
		if err := channelHelper.HandleShouldHitVendorOffError(msg); err != nil {
			utils.Error(fmt.Errorf("failed to handle shouldHitVendor off error: %v", err))
		}
		return buildSmsResult(msg, req, response)
	}

	switch msg.Vendor {
	case variables.TIMES:
		response = timesSms.HitTimesSmsApi(req)
	case variables.SINCH:
		response = sinchSms.HitSinchSmsApi(req)
	case variables.PINNACLE:
		response = pinnacleSms.HitPinnacleApi(req)
	default:
		response.Outcome = outcome.FailedFinal
		response.ResponseMessage = fmt.Sprintf("unsupported SMS vendor: %s", msg.Vendor)
	}

	// If the SMS is a regulated marketing SMS and the result is a compliance failure, then we need to count the compliance response
	complianceBlocked := strings.HasPrefix(response.ResponseMessage, "[WECREDIT_SMS_")
	if complianceBlocked {
		countComplianceResponse(response.ResponseMessage, msg)
	}

	if response.Outcome == "" {
		if response.IsSent {
			response.Outcome = outcome.Submitted
		} else {
			response.Outcome = outcome.Unknown
		}
	}
	response.IsSent = outcome.IsSentDerived(response.Outcome)

	if err := channelHelper.UpdateRedisTransactionId(msg, response.TransactionId); err != nil {
		utils.Error(fmt.Errorf("failed to update Redis transactionId: %v", err))
	}

	if response.Outcome == outcome.Submitted {
		utils.Info(fmt.Sprintf("SMS submitted for Process: %s CommId: %s via %s", msg.ProcessName, msg.CommId, msg.Vendor))

		if smspolicy.IsRegulatedMarketingSMS(msg.Source, msg.SourceRowId, msg.Channel) {
			metrics.Count("wecredit_sms_sent_total", msg.Vendor, msg.Client, 1)
		}
	}

	if !complianceBlocked {
		metrics.Count("SmsProviderAttempts", msg.Vendor, msg.Client, 1)
	}
	metrics.Count("SmsProviderOutcome_"+response.Outcome, msg.Vendor, msg.Client, 1)

	return buildSmsResult(msg, req, response)
}

func smsRequestFromMessage(msg sdkModels.CommApiRequestBody) extapimodels.SmsRequestBody {
	dltTemplateID, _ := strconv.ParseInt(strings.TrimSpace(msg.TemplateReference), 10, 64)
	return extapimodels.SmsRequestBody{
		Mobile:                 msg.Mobile,
		Process:                msg.ProcessName,
		Client:                 msg.Client,
		Channel:                msg.Channel,
		CommId:                 msg.CommId,
		Vendor:                 msg.Vendor,
		DltTemplateId:          dltTemplateID,
		EmiAmount:              msg.EmiAmount,
		CustomerName:           msg.CustomerName,
		LoanId:                 msg.LoanId,
		ApplicationNumber:      msg.ApplicationNumber,
		DueDate:                msg.DueDate,
		Description:            msg.Description,
		PaymentLink:            msg.PaymentLink,
		TemplateVariableValues: msg.TemplateVariableValues,
		Source:                 msg.Source,
		SourceRowId:            msg.SourceRowId,
		CampaignDate:           msg.CampaignDate,
	}
}

func complianceBlockedResult(msg sdkModels.CommApiRequestBody, req extapimodels.SmsRequestBody, decision smspolicy.Decision) (SendSmsResult, error) {
	response := extapimodels.SmsResponse{
		Outcome:         outcome.FailedFinal,
		ResponseMessage: decision.ErrorMessage(),
	}

	countComplianceDecision(decision, msg)
	utils.Info(fmt.Sprintf("WECREDIT_SMS_COMPLIANCE_BLOCKED sourceRowId=%d eventId=%s commId=%s campaignDate=%s currentIST=%s decision=%s",
		msg.SourceRowId, msg.EventId, msg.CommId, decision.CampaignDate,
		decision.CurrentIST.Format(time.RFC3339), decision.Code))

	return buildSmsResult(msg, req, response)
}

// TerminalReplayResult reconstructs the durable SMS output for an SQS
// redelivery whose provider result is already stored in Redis. It never calls a
// provider and lets the consumer repair a partially completed audit write.
func TerminalReplayResult(msg sdkModels.CommApiRequestBody, transactionID, errorMessage string) SendSmsResult {
	response := extapimodels.SmsResponse{
		TransactionId:   strings.TrimSpace(transactionID),
		ResponseMessage: strings.TrimSpace(errorMessage),
		Outcome:         outcome.FailedFinal,
	}

	if response.TransactionId != "" {
		response.Outcome = outcome.Submitted
		response.ResponseMessage = ""
	}

	result, _ := buildSmsResult(msg, smsRequestFromMessage(msg), response)
	return result
}

func countComplianceDecision(decision smspolicy.Decision, msg sdkModels.CommApiRequestBody) {
	switch decision.Code {
	case smspolicy.DecisionCutoff:
		metrics.Count("wecredit_sms_cutoff_blocked_total", msg.Vendor, msg.Client, 1)
		metrics.Count("wecredit_sms_retry_blocked_total", msg.Vendor, msg.Client, 1)

	case smspolicy.DecisionExpired:
		metrics.Count("wecredit_sms_expired_total", msg.Vendor, msg.Client, 1)
		metrics.Count("wecredit_sms_retry_blocked_total", msg.Vendor, msg.Client, 1)
	}
}

func countComplianceResponse(message string, msg sdkModels.CommApiRequestBody) {
	currentIST := smspolicy.IST(smspolicy.Now())
	utils.Info(fmt.Sprintf("WECREDIT_SMS_COMPLIANCE_BLOCKED sourceRowId=%d eventId=%s commId=%s campaignDate=%s currentIST=%s",
		msg.SourceRowId, msg.EventId, msg.CommId, msg.CampaignDate, currentIST.Format(time.RFC3339)))

	switch {
	case strings.Contains(message, smspolicy.DecisionCutoff):
		metrics.Count("wecredit_sms_cutoff_blocked_total", msg.Vendor, msg.Client, 1)
		metrics.Count("wecredit_sms_retry_blocked_total", msg.Vendor, msg.Client, 1)

	case strings.Contains(message, smspolicy.DecisionExpired):
		metrics.Count("wecredit_sms_expired_total", msg.Vendor, msg.Client, 1)
		metrics.Count("wecredit_sms_retry_blocked_total", msg.Vendor, msg.Client, 1)
	}
}

func buildSmsResult(msg sdkModels.CommApiRequestBody, req extapimodels.SmsRequestBody, response extapimodels.SmsResponse) (SendSmsResult, error) {
	response.DltTemplateId = req.DltTemplateId
	response.CommId = msg.CommId
	response.Vendor = msg.Vendor
	response.MobileNumber = msg.Mobile
	if response.Outcome == "" {
		response.Outcome = outcome.Unknown
	}
	response.IsSent = outcome.IsSentDerived(response.Outcome)
	if response.ResponseMessage == "" {
		response.ResponseMessage = response.Outcome
	} else if !strings.HasPrefix(response.ResponseMessage, "[") {
		response.ResponseMessage = fmt.Sprintf("[%s] %s", response.Outcome, response.ResponseMessage)
	}

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("mapping error: %v", err))
	}
	if dbMappedData == nil {
		dbMappedData = map[string]interface{}{}
	}

	raw, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("SMS Response: %s", string(raw)))

	return SendSmsResult{
		Processed: true,
		AckSQS:    outcome.IsTerminal(response.Outcome),
		DBData:    dbMappedData,
	}, nil
}
