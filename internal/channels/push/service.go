package push

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/push/fcm"
	"github.com/wecredit/communication-sdk/internal/metrics"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

const (
	providerName          = "FCM"
	maxParallelTokenSends = 10

	outcomeSubmitted      = "submitted"
	outcomeFailedFinal    = "failed_final"
	outcomeCancelledStale = "cancelled_stale"
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
	// InputAudit is one PushInputAuditTable row (nil when ShouldHitVendor off / no send).
	InputAudit map[string]interface{}
	// OutputAudits are PushOutputTable rows (one per token that reached a terminal FCM outcome).
	OutputAudits []map[string]interface{}
}

type Service struct {
	claims   tokenClaimStore
	executor retryExecutor
}

func NewService(claims tokenClaimStore, executor retryExecutor) (*Service, error) {
	if claims == nil {
		return nil, errors.New("PUSH token claim store is required")
	}
	if executor == nil {
		return nil, errors.New("PUSH retry executor is required")
	}
	return &Service{claims: claims, executor: executor}, nil
}

// Send resolves one template and fans out independently to each unique device
// token. Per-token Redis claims (EventId:fingerprint) are the dedupe authority.
// Audit maps are returned for the consumer to InsertData (SMS-style).
//
// ShouldHitVendor lives here (SMS-style inside the channel Send). AssignVendor
// lives in handlePush — do not duplicate it here.
func (s *Service) Send(ctx context.Context, request sdkModels.CommApiRequestBody) (Result, error) {
	if strings.TrimSpace(request.Vendor) == "" {
		request.Vendor = providerName
	}

	tokens := uniqueTokens(request.DeviceTokens)

	if !channelHelper.ShouldHitVendor(request.Client, request.Channel) {
		skipMsg := fmt.Sprintf(
			"shouldHitVendor is off for client=%s channel=%s eventId=%s",
			request.Client, request.Channel, request.EventId,
		)
		skipped, err := markShouldHitVendorOff(s.claims, request, tokens, skipMsg)
		if err != nil {
			return Result{Processed: false, AckSQS: false}, err
		}
		return Result{Processed: true, AckSQS: true, Skipped: skipped}, nil
	}

	title, body, templateName, resolvedVendor, err := resolveContent(request)
	if err != nil {
		return Result{}, err
	}

	if len(tokens) == 0 {
		return Result{}, errors.New("PUSH request contains no usable device tokens")
	}

	type tokenResult struct {
		outcome string
		skipped bool
		err     error
		output  map[string]interface{}
	}
	results := make(chan tokenResult, len(tokens))
	var workers sync.WaitGroup

	tokenJobs := make(chan string)
	workerCount := len(tokens)
	if workerCount > maxParallelTokenSends {
		workerCount = maxParallelTokenSends
	}
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for deviceToken := range tokenJobs {
				outcome, skipped, output, sendErr := s.sendTokenSafely(ctx, request, title, body, deviceToken)
				results <- tokenResult{outcome: outcome, skipped: skipped, output: output, err: sendErr}
			}
		}()
	}
	for _, token := range tokens {
		tokenJobs <- token
	}
	close(tokenJobs)

	workers.Wait()
	close(results)

	result := Result{
		Processed:  true,
		AckSQS:     true,
		InputAudit: buildInputAudit(request, resolvedVendor, templateName, len(tokens)),
	}
	var failures []error
	for tokenResult := range results {
		if tokenResult.err != nil {
			result.Processed = false
			result.AckSQS = false
			failures = append(failures, tokenResult.err)
			continue
		}
		if tokenResult.output != nil {
			result.OutputAudits = append(result.OutputAudits, tokenResult.output)
		}
		if tokenResult.skipped {
			result.Skipped++
			continue
		}
		switch tokenResult.outcome {
		case outcomeSubmitted:
			result.Submitted++
		case outcomeFailedFinal:
			result.FailedFinal++
		case outcomeCancelledStale:
			result.CancelledStale++
		default:
			result.Processed = false
			result.AckSQS = false
			failures = append(failures, fmt.Errorf("PUSH token ended in non-terminal status %q", tokenResult.outcome))
		}
	}

	return result, errors.Join(failures...)
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

func markShouldHitVendorOff(claims tokenClaimStore, request sdkModels.CommApiRequestBody, tokens []string, skipMsg string) (int, error) {
	if len(tokens) == 0 {
		if err := channelHelper.UpdateRedisErrorMessage(request, skipMsg); err != nil {
			return 0, fmt.Errorf("record ShouldHitVendor-off PUSH skip: %w", err)
		}
		return 1, nil
	}
	skipped := 0
	for _, token := range tokens {
		fp, err := FingerprintToken(token)
		if err != nil {
			return 0, fmt.Errorf("fingerprint ShouldHitVendor-off PUSH token: %w", err)
		}
		field := TokenRedisField(request, fp)
		skip, claimErr := claimTokenField(claims, request, field)
		if claimErr != nil {
			return 0, fmt.Errorf("claim ShouldHitVendor-off PUSH token: %w", claimErr)
		}
		if skip {
			skipped++
			continue
		}
		if err := claims.SetErrorMessage(field, skipMsg); err != nil {
			return 0, fmt.Errorf("record ShouldHitVendor-off PUSH token skip: %w", err)
		}
		skipped++
	}
	if skipped == 0 {
		return 1, nil
	}
	return skipped, nil
}

func (s *Service) sendTokenSafely(
	ctx context.Context,
	request sdkModels.CommApiRequestBody,
	title, body, deviceToken string,
) (outcome string, skipped bool, output map[string]interface{}, err error) {
	defer func() {
		if recover() != nil {
			outcome = ""
			skipped = false
			output = nil
			err = errors.New("PUSH token worker panic recovered")
		}
	}()
	return s.sendToken(ctx, request, title, body, deviceToken)
}

func (s *Service) sendToken(
	ctx context.Context,
	request sdkModels.CommApiRequestBody,
	title, body, deviceToken string,
) (string, bool, map[string]interface{}, error) {
	fingerprint, err := FingerprintToken(deviceToken)
	if err != nil {
		return "", false, nil, err
	}

	field := TokenRedisField(request, fingerprint)
	if replay, terminal, replayErr := terminalReplayOutput(s.claims, request, field, fingerprint); replayErr != nil {
		return "", false, nil, replayErr
	} else if terminal {
		return "", true, replay, nil
	}

	skip, err := claimTokenField(s.claims, request, field)
	if err != nil {
		return "", false, nil, err
	}
	if skip {
		return "", true, nil, nil
	}

	payload, err := fcm.BuildSendRequest(deviceToken, title, body, request)
	if err != nil {
		return s.finalizeToken(request, field, fingerprint, fcm.ExecutionResult{
			Outcome: fcm.OutcomeFailedFinal,
			Code:    "FCM_PAYLOAD_INVALID",
		})
	}

	observer := func(_ context.Context, attempt int) error {
		metrics.Count("PushProviderAttempts", providerName, request.Client, 1)
		return nil
	}

	// Before a second FCM attempt, confirm our blank claim was not reclaimed or finalized.
	guard := func(context.Context) (bool, error) {
		exists, txn, errMsg, getErr := s.claims.Get(field)
		if getErr != nil {
			return false, getErr
		}
		if !exists {
			return false, nil
		}
		if strings.TrimSpace(txn) != "" || strings.TrimSpace(errMsg) != "" {
			return false, nil
		}
		return true, nil
	}

	execution, err := s.executor.ExecuteWithObserver(ctx, request.Client, payload, guard, observer)
	if err != nil {
		return "", false, nil, err
	}
	return s.finalizeToken(request, field, fingerprint, execution)
}

// terminalReplayOutput rebuilds the audit row from a terminal Redis claim on
// SQS redelivery. This mirrors the SMS terminal-replay path: FCM is never sent
// again, but an earlier failed audit write gets another chance before ACK.
func terminalReplayOutput(
	claims tokenClaimStore,
	request sdkModels.CommApiRequestBody,
	field, fingerprint string,
) (map[string]interface{}, bool, error) {
	exists, transactionID, errorMessage, err := claims.Get(field)
	if err != nil {
		return nil, false, err
	}

	if !exists {
		return nil, false, nil
	}

	if transactionID = strings.TrimSpace(transactionID); transactionID != "" {
		return buildOutputAudit(request, fingerprint, outcomeSubmitted, fcm.ExecutionResult{
			Outcome:      fcm.OutcomeSubmitted,
			MessageID:    transactionID,
			AttemptCount: 0,
		}), true, nil
	}

	if errorMessage = strings.TrimSpace(errorMessage); errorMessage != "" {
		return buildOutputAudit(request, fingerprint, outcomeFailedFinal, fcm.ExecutionResult{
			Outcome:      fcm.OutcomeFailedFinal,
			Code:         errorMessage,
			AttemptCount: 0,
		}), true, nil
	}

	return nil, false, nil
}

func claimTokenField(claims tokenClaimStore, request sdkModels.CommApiRequestBody, field string) (skip bool, err error) {
	exists, txn, errMsg, err := claims.Get(field)
	if err != nil {
		return false, err
	}
	if exists {
		if strings.TrimSpace(txn) != "" || strings.TrimSpace(errMsg) != "" {
			return true, nil
		}

		if strings.TrimSpace(request.EventId) != "" {
			reclaimed, reclaimErr := claims.ReclaimBlank(field)
			if reclaimErr != nil {
				return false, reclaimErr
			}

			if !reclaimed {
				return true, nil
			}
		} else {
			return true, nil
		}
	}

	if err := claims.Claim(field); err != nil {
		// HSetNX race: another worker already claimed → skip send (same as SMS).
		// Any other Redis error must surface so the message is not Ack'd.
		if strings.Contains(err.Error(), "already exists") {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (s *Service) finalizeToken(
	request sdkModels.CommApiRequestBody,
	field, fingerprint string,
	execution fcm.ExecutionResult,
) (string, bool, map[string]interface{}, error) {
	outcome, err := mapOutcome(execution.Outcome)
	if err != nil {
		return "", false, nil, err
	}

	switch outcome {
	case outcomeSubmitted:
		if err := s.claims.SetTransactionID(field, execution.MessageID); err != nil {
			return "", false, nil, err
		}
	default:
		msg := execution.Code
		if strings.TrimSpace(msg) == "" {
			msg = string(execution.Outcome)
		}
		if err := s.claims.SetErrorMessage(field, msg); err != nil {
			return "", false, nil, err
		}
	}

	metrics.Count("PushProviderOutcome_"+outcome, providerName, request.Client, 1)
	return outcome, false, buildOutputAudit(request, fingerprint, outcome, execution), nil
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
