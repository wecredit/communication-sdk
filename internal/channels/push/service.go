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
	"github.com/wecredit/communication-sdk/sdk/utils"
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
		skipped := markShouldHitVendorOff(s.claims, request, tokens, skipMsg)
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
		outcome  string
		skipped  bool
		err      error
		output   map[string]interface{}
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
		InputAudit: buildInputAudit(request, resolvedVendor, templateName, title, body, len(tokens)),
	}
	var failures []error
	for tokenResult := range results {
		if tokenResult.err != nil {
			result.Processed = false
			result.AckSQS = false
			failures = append(failures, tokenResult.err)
			continue
		}
		if tokenResult.skipped {
			result.Skipped++
			continue
		}
		if tokenResult.output != nil {
			result.OutputAudits = append(result.OutputAudits, tokenResult.output)
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
	vendor, templateName, title, body string,
	deviceCount int,
) map[string]interface{} {
	return map[string]interface{}{
		"CommId":            strings.TrimSpace(request.CommId),
		"EventId":           strings.TrimSpace(request.EventId),
		"Client":            strings.TrimSpace(request.Client),
		"ProcessName":       strings.TrimSpace(request.ProcessName),
		"Stage":             request.Stage,
		"Vendor":            strings.TrimSpace(vendor),
		"TemplateName":      strings.TrimSpace(templateName),
		"Title":             title,
		"Body":              body,
		"NotificationEvent": strings.TrimSpace(request.NotificationEvent),
		"DeepLink":          strings.TrimSpace(request.DeepLink),
		"UserId":            strings.TrimSpace(request.UserId),
		"ApplicationNumber": strings.TrimSpace(request.ApplicationNumber),
		"CampaignDate":      strings.TrimSpace(request.CampaignDate),
		"DeviceCount":       deviceCount,
		"CreatedOn":         time.Now().UTC(),
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

func markShouldHitVendorOff(claims tokenClaimStore, request sdkModels.CommApiRequestBody, tokens []string, skipMsg string) int {
	if len(tokens) == 0 {
		if err := channelHelper.UpdateRedisErrorMessage(request, skipMsg); err != nil {
			utils.Error(fmt.Errorf("failed to handle shouldHitVendor off for PUSH: %v", err))
		}
		return 1
	}
	skipped := 0
	for _, token := range tokens {
		fp, err := FingerprintToken(token)
		if err != nil {
			utils.Error(fmt.Errorf("PUSH fingerprint failed during ShouldHitVendor-off: %v", err))
			continue
		}
		field := TokenRedisField(request, fp)
		skip, claimErr := claimTokenField(claims, request, field)
		if claimErr != nil {
			utils.Error(fmt.Errorf("PUSH ShouldHitVendor-off claim failed: %v", claimErr))
			continue
		}
		if skip {
			skipped++
			continue
		}
		if err := claims.SetErrorMessage(field, skipMsg); err != nil {
			utils.Error(fmt.Errorf("PUSH ShouldHitVendor-off redis update failed: %v", err))
		}
		skipped++
	}
	if skipped == 0 {
		return 1
	}
	return skipped
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

	skip, err := claimTokenField(s.claims, request, field)
	if err != nil {
		return "", false, nil, err
	}
	if skip {
		return "", true, nil, nil
	}

	payload, err := fcm.BuildDataOnlyRequest(deviceToken, title, body, request)
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

	execution, err := s.executor.ExecuteWithObserver(ctx, request.Client, payload, nil, observer)
	if err != nil {
		return "", false, nil, err
	}
	return s.finalizeToken(request, field, fingerprint, execution)
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
		return true, nil
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
	firstName := strings.TrimSpace(request.CustomerName)
	return strings.NewReplacer(
		"{{firstName}}", firstName,
		"{{ firstName }}", firstName,
	).Replace(value)
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
