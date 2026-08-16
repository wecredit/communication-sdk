package sms

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	pinnacleSms "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle"
	sinchSms "github.com/wecredit/communication-sdk/internal/channels/sms/sinch"
	timesSms "github.com/wecredit/communication-sdk/internal/channels/sms/times"
	"github.com/wecredit/communication-sdk/internal/metrics"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
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

	req := extapimodels.SmsRequestBody{
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
	}
	channelHelper.PopulateSmsFields(&req, templateData)

	var response extapimodels.SmsResponse
	shouldHitVendor := channelHelper.ShouldHitVendor(msg.Client, msg.Channel)
	utils.Debug(fmt.Sprintf("Channel: %s Mobile: %s, Should hit vendor: %v\n", msg.Channel, msg.Mobile, shouldHitVendor))

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
	}
	metrics.Count("SmsProviderAttempts", msg.Vendor, msg.Client, 1)
	metrics.Count("SmsProviderOutcome_"+response.Outcome, msg.Vendor, msg.Client, 1)

	return buildSmsResult(msg, req, response)
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
