package push

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/push/audit"
	"github.com/wecredit/communication-sdk/internal/channels/push/fcm"
	"github.com/wecredit/communication-sdk/internal/channels/push/ledger"
	"github.com/wecredit/communication-sdk/internal/metrics"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

const (
	providerName          = "FCM"
	maxParallelTokenSends = 10
)

type ledgerStore interface {
	CreateOrGetPending(context.Context, ledger.Identity, string) (ledger.Dispatch, bool, error)
	Claim(context.Context, uint64) (ledger.Dispatch, bool, error)
	IsClaimCurrent(context.Context, uint64) (bool, time.Duration, error)
	RecordAttempt(context.Context, uint64, int) (bool, error)
	Finalize(context.Context, uint64, ledger.Status, int, string, string) (bool, error)
}

type retryExecutor interface {
	ExecuteWithObserver(context.Context, string, fcm.SendRequest, fcm.RetryGuard, fcm.AttemptObserver) (fcm.ExecutionResult, error)
}

type Result struct {
	Processed      bool
	AckSQS         bool
	Submitted      int
	FailedFinal    int
	CancelledStale int
	Skipped        int
}

type Service struct {
	ledger   ledgerStore
	executor retryExecutor
}

func NewService(store ledgerStore, executor retryExecutor) (*Service, error) {
	if store == nil {
		return nil, errors.New("PUSH ledger store is required")
	}
	if executor == nil {
		return nil, errors.New("PUSH retry executor is required")
	}
	return &Service{ledger: store, executor: executor}, nil
}

// Send resolves one template and fans out independently to each unique device
// token. A token is acknowledged only after its ledger row is terminal.
func (s *Service) Send(ctx context.Context, request sdkModels.CommApiRequestBody) (Result, error) {
	if strings.TrimSpace(request.Vendor) == "" {
		request.Vendor = providerName
	}
	title, body, templateName, resolvedVendor, err := resolveContent(request)
	if err != nil {
		return Result{}, err
	}

	tokens := uniqueTokens(request.DeviceTokens)
	if len(tokens) == 0 {
		return Result{}, errors.New("PUSH request contains no usable device tokens")
	}

	if !audit.TrySubmitInput(audit.Input{
		CommID:            request.CommId,
		EventID:           request.EventId,
		Client:            request.Client,
		ProcessName:       request.ProcessName,
		Stage:             request.Stage,
		Vendor:            resolvedVendor,
		TemplateName:      templateName,
		Title:             title,
		Body:              body,
		NotificationEvent: request.NotificationEvent,
		DeepLink:          request.DeepLink,
		UserID:            request.UserId,
		ApplicationNumber: request.ApplicationNumber,
		DeviceCount:       len(tokens),
	}) {
		utils.Warn(fmt.Sprintf("PUSH input audit queue unavailable client=%s eventId=%s", request.Client, request.EventId))
	}

	identity := ledgerIdentity(request, templateName)
	type tokenResult struct {
		status  ledger.Status
		skipped bool
		err     error
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
				status, skipped, sendErr := s.sendTokenSafely(ctx, request, identity, title, body, deviceToken)
				results <- tokenResult{status: status, skipped: skipped, err: sendErr}
			}
		}()
	}
	for _, token := range tokens {
		tokenJobs <- token
	}
	close(tokenJobs)

	workers.Wait()
	close(results)

	result := Result{Processed: true, AckSQS: true}
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
		switch tokenResult.status {
		case ledger.StatusSubmitted:
			result.Submitted++
		case ledger.StatusFailedFinal:
			result.FailedFinal++
		case ledger.StatusCancelledStale:
			result.CancelledStale++
		default:
			result.Processed = false
			result.AckSQS = false
			failures = append(failures, fmt.Errorf("PUSH token ended in non-terminal status %q", tokenResult.status))
		}
	}

	return result, errors.Join(failures...)
}

func (s *Service) sendTokenSafely(
	ctx context.Context,
	request sdkModels.CommApiRequestBody,
	identity ledger.Identity,
	title, body, deviceToken string,
) (status ledger.Status, skipped bool, err error) {
	defer func() {
		if recover() != nil {
			status = ""
			skipped = false
			err = errors.New("PUSH token worker panic recovered")
		}
	}()
	return s.sendToken(ctx, request, identity, title, body, deviceToken)
}

func (s *Service) sendToken(
	ctx context.Context,
	request sdkModels.CommApiRequestBody,
	identity ledger.Identity,
	title, body, deviceToken string,
) (ledger.Status, bool, error) {
	dispatch, _, err := s.ledger.CreateOrGetPending(ctx, identity, deviceToken)
	if err != nil {
		return "", false, err
	}
	if dispatch.Status.Terminal() {
		return dispatch.Status, true, nil
	}

	dispatch, claimed, err := s.ledger.Claim(ctx, dispatch.ID)
	if err != nil {
		return "", false, err
	}
	if !claimed {
		return "", false, errors.New("PUSH ledger row is already claimed")
	}
	if dispatch.ReclaimCount > 0 {
		metrics.Count("PushClaimReclaimed", providerName, request.Client, 1)
	}

	current, claimAge, err := s.ledger.IsClaimCurrent(ctx, dispatch.ID)
	if err != nil {
		return "", false, err
	}
	emitClaimAge(request.Client, claimAge)
	if !current {
		return "", false, errors.New("PUSH claim expired before initial provider attempt")
	}

	payload, err := fcm.BuildDataOnlyRequest(deviceToken, title, body, request)
	if err != nil {
		return s.finalize(ctx, request, dispatch, fcm.ExecutionResult{
			Outcome: fcm.OutcomeFailedFinal,
			Code:    "FCM_PAYLOAD_INVALID",
		})
	}

	guard := func(guardCtx context.Context) (bool, error) {
		owned, age, guardErr := s.ledger.IsClaimCurrent(guardCtx, dispatch.ID)
		emitClaimAge(request.Client, age)
		return owned, guardErr
	}
	observer := func(attemptCtx context.Context, attempt int) error {
		recorded, recordErr := s.ledger.RecordAttempt(attemptCtx, dispatch.ID, attempt)
		if recordErr != nil {
			return recordErr
		}
		if !recorded {
			return errors.New("PUSH claim expired before provider attempt")
		}
		metrics.Count("PushProviderAttempts", providerName, request.Client, 1)
		return nil
	}

	execution, err := s.executor.ExecuteWithObserver(ctx, request.Client, payload, guard, observer)
	if err != nil {
		return "", false, err
	}
	return s.finalize(ctx, request, dispatch, execution)
}

func (s *Service) finalize(
	ctx context.Context,
	request sdkModels.CommApiRequestBody,
	dispatch ledger.Dispatch,
	execution fcm.ExecutionResult,
) (ledger.Status, bool, error) {
	status, err := ledgerStatus(execution.Outcome)
	if err != nil {
		return "", false, err
	}

	finalized, err := s.ledger.Finalize(ctx, dispatch.ID, status, execution.AttemptCount, execution.Code, execution.MessageID)
	if err != nil {
		return "", false, err
	}
	if !finalized {
		return "", false, errors.New("PUSH ledger claim was lost before finalization")
	}

	metrics.Count("PushProviderOutcome_"+string(status), providerName, request.Client, 1)
	if !audit.TrySubmitOutput(audit.Output{
		LedgerID:          dispatch.ID,
		CommID:            request.CommId,
		EventID:           request.EventId,
		Client:            request.Client,
		TokenFingerprint:  dispatch.TokenFingerprint,
		Outcome:           string(status),
		AttemptCount:      execution.AttemptCount,
		ErrorCode:         execution.Code,
		ProviderMessageID: execution.MessageID,
	}) {
		utils.Warn(fmt.Sprintf("PUSH output audit queue unavailable client=%s eventId=%s ledgerId=%d", request.Client, request.EventId, dispatch.ID))
	}

	return status, false, nil
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

func ledgerIdentity(request sdkModels.CommApiRequestBody, templateName string) ledger.Identity {
	variant := strings.TrimSpace(request.TemplateReference)
	if variant == "" {
		variant = strings.TrimSpace(templateName)
	}
	return ledger.Identity{
		Client:              request.Client,
		EventID:             request.EventId,
		Campaign:            request.ProcessName,
		CampaignDate:        request.CampaignDate,
		Variant:             variant,
		EligibilityIdentity: request.EventId,
	}
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

func ledgerStatus(outcome fcm.Outcome) (ledger.Status, error) {
	switch outcome {
	case fcm.OutcomeSubmitted:
		return ledger.StatusSubmitted, nil
	case fcm.OutcomeFailedFinal:
		return ledger.StatusFailedFinal, nil
	case fcm.OutcomeCancelledStale:
		return ledger.StatusCancelledStale, nil
	default:
		return "", fmt.Errorf("cannot finalize retryable PUSH outcome %q", outcome)
	}
}

func emitClaimAge(client string, age time.Duration) {
	if age < 0 {
		return
	}
	seconds := int(age / time.Second)
	metrics.Value("PushClaimAgeSeconds", "Seconds", providerName, client, float64(seconds))
	if age >= ledger.ClaimAgeWarning {
		metrics.Count("PushClaimAgeWarning", providerName, client, 1)
		utils.Warn(fmt.Sprintf("PUSH claim age warning client=%s ageSeconds=%d", client, seconds))
	}
}
