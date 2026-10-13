package push

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/push/fcm"
	"github.com/wecredit/communication-sdk/internal/metrics"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

const (
	providerName = "FCM"

	outcomeSubmitted      = "submitted"
	outcomeFailedFinal    = "failed_final"
	outcomeCancelledStale = "cancelled_stale"
	outcomeSkipped        = "skipped"
)

type retryExecutor interface {
	ExecuteWithObserver(context.Context, string, fcm.SendRequest, fcm.RetryGuard, fcm.AttemptObserver) (fcm.ExecutionResult, error)
}

// Result mirrors SMS SendSmsResult shape: consumer owns InsertData for audits.
type Result struct {
	Processed      bool
	AckSQS         bool
	Submitted      int
	FailedFinal    int
	CancelledStale int
	Skipped        int
	// InputAudit is one PushInputAuditTable row. It is set for a vendor send and
	// for a ShouldHitVendor-off skip, matching SMS, WhatsApp, and RCS.
	InputAudit map[string]interface{}
	// OutputAudits are PushOutputTable rows: one per token that reached a
	// terminal FCM outcome, or one per token skipped because ShouldHitVendor is off.
	// FreshOutputAudits is the same set. The consumer inserts those rows once.
	OutputAudits      []map[string]interface{}
	FreshOutputAudits []map[string]interface{}
	Replayed          int
}

type Service struct {
	executor retryExecutor
}

func NewService(executor retryExecutor) (*Service, error) {
	if executor == nil {
		return nil, errors.New("PUSH retry executor is required")
	}
	return &Service{executor: executor}, nil
}

// Send resolves one template and sends each unique device token once.
// The EventId claim in front of the queue handler is the same gate WhatsApp
// and RCS use. This path does not take another Redis claim per token.
//
// ShouldHitVendor lives here. AssignVendor lives in handlePush.
func (s *Service) Send(ctx context.Context, request sdkModels.CommApiRequestBody) (Result, error) {
	if strings.TrimSpace(request.Vendor) == "" {
		request.Vendor = providerName
	}

	tokens := uniqueTokens(request.DeviceTokens)

	if !channelHelper.ShouldHitVendor(request.Client, request.Channel) {
		return skippedVendorOff(request, tokens), nil
	}

	title, body, templateName, resolvedVendor, err := resolveContent(request)
	if err != nil {
		return Result{}, err
	}

	if len(tokens) == 0 {
		return Result{}, errors.New("PUSH request contains no usable device tokens")
	}

	result := Result{
		Processed:         true,
		AckSQS:            true,
		InputAudit:        buildInputAudit(request, resolvedVendor, templateName, len(tokens)),
		OutputAudits:      make([]map[string]interface{}, 0, len(tokens)),
		FreshOutputAudits: make([]map[string]interface{}, 0, len(tokens)),
	}
	var failures []error
	for _, deviceToken := range tokens {
		output, outcome, sendErr := s.sendToken(ctx, request, title, body, deviceToken)
		if sendErr != nil {
			result.Processed = false
			result.AckSQS = false
			failures = append(failures, sendErr)
			continue
		}
		if output != nil {
			result.OutputAudits = append(result.OutputAudits, output)
			result.FreshOutputAudits = append(result.FreshOutputAudits, output)
		}
		switch outcome {
		case outcomeSubmitted:
			result.Submitted++
		case outcomeFailedFinal:
			result.FailedFinal++
		case outcomeCancelledStale:
			result.CancelledStale++
		default:
			result.Processed = false
			result.AckSQS = false
			failures = append(failures, fmt.Errorf("PUSH token ended in non-terminal status %q", outcome))
		}
	}

	return result, errors.Join(failures...)
}

func skippedVendorOff(request sdkModels.CommApiRequestBody, tokens []string) Result {
	templateName := ""
	vendor := strings.TrimSpace(request.Vendor)
	if _, _, resolvedName, resolvedVendor, resolveErr := resolveContent(request); resolveErr == nil {
		templateName = resolvedName
		if strings.TrimSpace(resolvedVendor) != "" {
			vendor = resolvedVendor
		}
	}

	skipped := len(tokens)
	if skipped == 0 {
		skipped = 1
	}

	result := Result{
		Processed:         true,
		AckSQS:            true,
		Skipped:           skipped,
		InputAudit:        buildInputAudit(request, vendor, templateName, len(tokens)),
		OutputAudits:      make([]map[string]interface{}, 0, len(tokens)),
		FreshOutputAudits: make([]map[string]interface{}, 0, len(tokens)),
	}

	skipMsg := vendorOffMessage(request.Mobile)
	for _, token := range tokens {
		fingerprint, err := FingerprintToken(token)
		if err != nil {
			continue
		}
		output := buildOutputAudit(request, fingerprint, outcomeSkipped, fcm.ExecutionResult{Code: skipMsg})
		result.OutputAudits = append(result.OutputAudits, output)
		result.FreshOutputAudits = append(result.FreshOutputAudits, output)
	}

	return result
}

func buildInputAudit(
	request sdkModels.CommApiRequestBody,
	vendor, templateName string,
	deviceCount int,
) map[string]interface{} {
	return map[string]interface{}{
		"CommId":       strings.TrimSpace(request.CommId),
		"EventId":      strings.TrimSpace(request.EventId),
		"Client":       strings.TrimSpace(request.Client),
		"ProcessName":  strings.TrimSpace(request.ProcessName),
		"Stage":        request.Stage,
		"Vendor":       strings.TrimSpace(vendor),
		"TemplateName": strings.TrimSpace(templateName),
		"DeviceCount":  deviceCount,
		"CreatedOn":    time.Now().UTC(),
	}
}

func buildOutputAudit(
	request sdkModels.CommApiRequestBody,
	fingerprint, outcome string,
	execution fcm.ExecutionResult,
) map[string]interface{} {
	return map[string]interface{}{
		"CommId":            strings.TrimSpace(request.CommId),
		"EventId":           strings.TrimSpace(request.EventId),
		"Client":            strings.TrimSpace(request.Client),
		"TokenFingerprint":  fingerprint,
		"Outcome":           outcome,
		"AttemptCount":      execution.AttemptCount,
		"ErrorCode":         strings.TrimSpace(execution.Code),
		"ProviderMessageId": strings.TrimSpace(execution.MessageID),
		"CreatedOn":         time.Now().UTC(),
	}
}

func (s *Service) sendToken(
	ctx context.Context,
	request sdkModels.CommApiRequestBody,
	title, body, deviceToken string,
) (map[string]interface{}, string, error) {
	fingerprint, err := FingerprintToken(deviceToken)
	if err != nil {
		return nil, "", err
	}

	payload, err := fcm.BuildSendRequest(deviceToken, title, body, request)
	if err != nil {
		execution := fcm.ExecutionResult{Outcome: fcm.OutcomeFailedFinal, Code: "FCM_PAYLOAD_INVALID"}
		return buildOutputAudit(request, fingerprint, outcomeFailedFinal, execution), outcomeFailedFinal, nil
	}

	execution, err := s.executor.ExecuteWithObserver(ctx, request.Client, payload, nil, nil)
	if err != nil {
		return nil, "", err
	}

	outcome, err := mapOutcome(execution.Outcome)
	if err != nil {
		return nil, "", err
	}

	metrics.Count("PushProviderAttempts", providerName, request.Client, execution.AttemptCount)
	metrics.Count("PushProviderOutcome_"+outcome, providerName, request.Client, 1)
	return buildOutputAudit(request, fingerprint, outcome, execution), outcome, nil
}

// vendorOffMessage matches the SMS, WhatsApp, and RCS skip text. PushOutputTable.ErrorCode
// is varchar(64), so the value is capped at that length.
func vendorOffMessage(mobile string) string {
	message := strings.TrimSpace("shouldHitVendor is off for mobile " + strings.TrimSpace(mobile))
	if len(message) > 64 {
		return message[:64]
	}
	return message
}

func resolveContent(request sdkModels.CommApiRequestBody) (title, body, templateName, vendor string, err error) {
	applicationCache := cache.GetCache()
	if applicationCache == nil {
		return "", "", "", "", errors.New("PUSH template cache is not initialized")
	}

	templateDetails, found := applicationCache.GetMappedData(cache.TemplateDetailsData)
	if !found {
		return "", "", "", "", errors.New("PUSH template data not found in cache")
	}

	template, vendor, err := channelHelper.ResolveTemplateData(request, templateDetails)
	if err != nil {
		return "", "", "", "", fmt.Errorf("resolve PUSH template: %w", err)
	}

	title, _ = template["TemplateHeader"].(string)
	body, _ = template["TemplateText"].(string)
	templateName, _ = template["TemplateName"].(string)
	title = applyKnownVariables(title, request)
	body = applyKnownVariables(body, request)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
		return "", "", "", "", errors.New("PUSH template title and body are required")
	}

	if strings.TrimSpace(vendor) == "" {
		vendor = providerName
	}

	return title, body, templateName, vendor, nil
}

func applyKnownVariables(value string, request sdkModels.CommApiRequestBody) string {
	variables := map[string]string{
		"firstName":          request.CustomerName,
		"CustomerName":       request.CustomerName,
		"LoanId":             request.LoanId,
		"ApplicationNumber":  request.ApplicationNumber,
		"DueDate":            request.DueDate,
		"EmiAmount":          request.EmiAmount,
		"PaymentLink":        request.PaymentLink,
		"TotalPayableAmount": request.TotalPayableAmount,
		"TodayPayableAmount": request.TodayPayableAmount,
		"SavingAmount":       request.SavingAmount,
		"BounceCharge":       request.BounceCharge,
	}

	replacements := make([]string, 0, len(variables)*2)
	for key, rawValue := range variables {
		value := strings.TrimSpace(rawValue)
		replacements = append(replacements, "{{"+key+"}}", value, "{{ "+key+" }}", value)
	}

	return strings.NewReplacer(replacements...).Replace(value)
}

// PushIdentity is attempt metadata for eligibility/audit fields (not a SQL ledger).
type PushIdentity struct {
	Client              string
	EventID             string
	Campaign            string
	CampaignDate        string
	Variant             string
	EligibilityIdentity string
}

func PushIdentityFromRequest(request sdkModels.CommApiRequestBody, templateName string) PushIdentity {
	variant := strings.TrimSpace(request.TemplateReference)
	if variant == "" {
		variant = strings.TrimSpace(templateName)
	}
	return PushIdentity{
		Client:              request.Client,
		EventID:             request.EventId,
		Campaign:            request.ProcessName,
		CampaignDate:        request.CampaignDate,
		Variant:             variant,
		EligibilityIdentity: EligibilityIdentity(request),
	}
}

func EligibilityIdentity(request sdkModels.CommApiRequestBody) string {
	if userID := strings.TrimSpace(request.UserId); userID != "" {
		return userID
	}
	return request.EventId
}

func uniqueTokens(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	unique := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		unique = append(unique, token)
	}
	return unique
}

func mapOutcome(outcome fcm.Outcome) (string, error) {
	switch outcome {
	case fcm.OutcomeSubmitted:
		return outcomeSubmitted, nil
	case fcm.OutcomeFailedFinal:
		return outcomeFailedFinal, nil
	case fcm.OutcomeCancelledStale:
		return outcomeCancelledStale, nil
	default:
		return "", fmt.Errorf("cannot finalize retryable PUSH outcome %q", outcome)
	}
}
