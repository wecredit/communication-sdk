package push

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/push/fcm"
	"github.com/wecredit/communication-sdk/internal/metrics"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

const (
	providerName = "FCM"

	// EventClaimLease is how long a live PUSH worker keeps the EventId claim.
	// A redelivery reclaims it only after this age, so the FCM retry wait cannot
	// be stolen and sent a second time. One FCM attempt is bounded by 30s.
	EventClaimLease = 2 * time.Minute

	outcomeSubmitted      = "submitted"
	outcomeFailedFinal    = "failed_final"
	outcomeCancelledStale = "cancelled_stale"
	outcomeSkipped        = "skipped"
)

var errClaimLost = errors.New("PUSH event claim is no longer current")

// TerminalStore records a device that already reached a terminal FCM outcome.
// A later delivery of the same EventId must not submit that device again.
type TerminalStore interface {
	Lookup(field string) (outcome, transactionID, errorMessage string, found bool, err error)
	Mark(field, outcome, transactionID, errorMessage string) error
}

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
	// Superseded means a redelivery owns the EventId claim. The handler must
	// not seal or release that claim.
	Superseded bool
}

type Service struct {
	executor   retryExecutor
	terminals  TerminalStore
	claimCheck func(claimID, redisKey string) (bool, error)
}

func NewService(executor retryExecutor) (*Service, error) {
	if executor == nil {
		return nil, errors.New("PUSH retry executor is required")
	}
	return &Service{executor: executor}, nil
}

// Send resolves one template and sends each unique device token once.
// The handler claims EventId on every consumer queue before this runs, and
// stamps a claim id so a redelivery cannot send the FCM retry. A device that
// already has a terminal outcome is not submitted again.
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
		output, outcome, fresh, sendErr := s.sendToken(ctx, request, title, body, deviceToken)
		if sendErr != nil {
			// The batch is unfinished, so the handler retries it. Devices that
			// already reached FCM stay noted and are not submitted again.
			result.Processed = false
			result.AckSQS = false
			failures = append(failures, sendErr)
			continue
		}
		if outcome == outcomeCancelledStale {
			result.Superseded = true
			result.CancelledStale++
			continue
		}
		if output != nil {
			result.OutputAudits = append(result.OutputAudits, output)
			if fresh {
				result.FreshOutputAudits = append(result.FreshOutputAudits, output)
			} else {
				result.Replayed++
			}
		}
		switch outcome {
		case outcomeSubmitted:
			result.Submitted++
		case outcomeFailedFinal:
			result.FailedFinal++
		case outcomeSkipped:
			result.Skipped++
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
) (map[string]interface{}, string, bool, error) {
	fingerprint, err := FingerprintToken(deviceToken)
	if err != nil {
		return nil, "", false, err
	}

	if output, outcome, found, replayErr := s.replayTerminal(request, fingerprint); replayErr != nil {
		return nil, "", false, replayErr
	} else if found {
		return output, outcome, false, nil
	}

	payload, err := fcm.BuildSendRequest(deviceToken, title, body, request)
	if err != nil {
		execution := fcm.ExecutionResult{Outcome: fcm.OutcomeFailedFinal, Code: "FCM_PAYLOAD_INVALID"}
		return buildOutputAudit(request, fingerprint, outcomeFailedFinal, execution), outcomeFailedFinal, true, nil
	}

	claimID := EventClaimID(ctx)
	redisKey := channelHelper.GenerateRedisKeyForRequest(request)
	var guard fcm.RetryGuard
	var observer fcm.AttemptObserver
	if claimID != "" {
		guard = func(context.Context) (bool, error) {
			return s.claimStillCurrent(claimID, redisKey)
		}
		observer = func(context.Context, int) error {
			return s.refreshEventClaim(claimID, redisKey)
		}
	}

	execution, err := s.executor.ExecuteWithObserver(ctx, request.Client, payload, guard, observer)
	if err != nil {
		if claimNoLongerCurrent(err) {
			return nil, outcomeCancelledStale, false, nil
		}
		return nil, "", false, err
	}

	outcome, err := mapOutcome(execution.Outcome)
	if err != nil {
		return nil, "", false, err
	}
	if outcome == outcomeCancelledStale {
		return nil, outcomeCancelledStale, false, nil
	}

	metrics.Count("PushProviderAttempts", providerName, request.Client, execution.AttemptCount)
	metrics.Count("PushProviderOutcome_"+outcome, providerName, request.Client, 1)
	return buildOutputAudit(request, fingerprint, outcome, execution), outcome, true, nil
}

func (s *Service) replayTerminal(request sdkModels.CommApiRequestBody, fingerprint string) (map[string]interface{}, string, bool, error) {
	store := s.terminalStore()
	if store == nil {
		return nil, "", false, nil
	}

	outcome, transactionID, errorMessage, found, err := store.Lookup(TokenRedisField(request, fingerprint))
	if err != nil || !found {
		return nil, "", false, err
	}

	if !rememberOutcome(outcome) {
		if strings.TrimSpace(transactionID) != "" {
			outcome = outcomeSubmitted
		} else {
			outcome = outcomeFailedFinal
		}
	}

	output := buildOutputAudit(request, fingerprint, outcome, fcm.ExecutionResult{
		MessageID: transactionID,
		Code:      errorMessage,
	})

	return output, outcome, true, nil
}

func (s *Service) terminalStore() TerminalStore {
	if s.terminals != nil {
		return s.terminals
	}

	store, err := newRedisTokenClaims()
	if err != nil {
		return nil
	}

	return store
}

// NoteFreshTerminals stores devices whose audits were just written, so a retry
// of an unfinished batch does not submit them again.
func NoteFreshTerminals(request sdkModels.CommApiRequestBody, outputs []map[string]interface{}) error {
	runtimeMu.RLock()
	service := runtimeService
	runtimeMu.RUnlock()
	if service == nil {
		return errors.New("PUSH service is not initialized")
	}

	return service.NoteFreshTerminals(request, outputs)
}

// NoteFreshTerminals stores terminal device outcomes for this service.
func (s *Service) NoteFreshTerminals(request sdkModels.CommApiRequestBody, outputs []map[string]interface{}) error {
	store := s.terminalStore()
	if store == nil {
		return errors.New("PUSH terminal store is not available")
	}

	var noteErr error
	for _, output := range outputs {
		outcome, _ := output["Outcome"].(string)
		if !rememberOutcome(outcome) {
			continue
		}

		fingerprint, _ := output["TokenFingerprint"].(string)
		fingerprint = strings.TrimSpace(fingerprint)
		if fingerprint == "" {
			continue
		}

		transactionID, _ := output["ProviderMessageId"].(string)
		errorMessage, _ := output["ErrorCode"].(string)

		if err := store.Mark(TokenRedisField(request, fingerprint), outcome, strings.TrimSpace(transactionID), strings.TrimSpace(errorMessage)); err != nil {
			noteErr = errors.Join(noteErr, err)
		}
	}

	return noteErr
}

// UseTerminalStore sets the device-outcome store. Production uses Redis when
// this is left unset.
func (s *Service) UseTerminalStore(store TerminalStore) {
	s.terminals = store
}

// UseClaimCheck replaces the EventId ownership read. Production reads Redis.
func (s *Service) UseClaimCheck(check func(claimID, redisKey string) (bool, error)) {
	s.claimCheck = check
}

func (s *Service) claimStillCurrent(claimID, redisKey string) (bool, error) {
	if s.claimCheck != nil {
		return s.claimCheck(claimID, redisKey)
	}

	return eventClaimStillCurrent(claimID, redisKey)
}

func (s *Service) refreshEventClaim(claimID, redisKey string) error {
	if s.claimCheck != nil {
		current, err := s.claimCheck(claimID, redisKey)
		if err != nil {
			return err
		}

		if !current {
			return errClaimLost
		}

		return nil
	}

	if redis.RDB == nil {
		return errors.New("PUSH redis client is nil")
	}

	return redis.RefreshPushClaim(redis.RDB, config.Configs.CommIdempotentKey, redisKey, claimID)
}

func eventClaimStillCurrent(claimID, redisKey string) (bool, error) {
	if redis.RDB == nil {
		return false, errors.New("PUSH redis client is nil")
	}

	hash := config.Configs.CommIdempotentKey
	current, exists, err := redis.GetPushClaimID(hash, redisKey, redis.RDB)
	if err != nil {
		return false, err
	}

	if !exists || current != claimID {
		return false, nil
	}

	_, transactionID, errorMessage, err := redis.GetMobileDataFromRedis(hash, redisKey, redis.RDB)
	if err != nil {
		return false, err
	}

	if strings.TrimSpace(transactionID) != "" || strings.TrimSpace(errorMessage) != "" {
		return false, nil
	}

	return true, nil
}

func claimNoLongerCurrent(err error) bool {
	return err != nil && (errors.Is(err, errClaimLost) || strings.Contains(err.Error(), "no longer owned"))
}

func rememberOutcome(outcome string) bool {
	switch outcome {
	case outcomeSubmitted, outcomeFailedFinal, outcomeSkipped:
		return true
	default:
		return false
	}
}

type eventClaimContextKey struct{}

// WithEventClaim attaches the EventId claim id the handler just stamped.
func WithEventClaim(ctx context.Context, claimID string) context.Context {
	claimID = strings.TrimSpace(claimID)
	if ctx == nil || claimID == "" {
		return ctx
	}

	return context.WithValue(ctx, eventClaimContextKey{}, claimID)
}

// EventClaimID returns the claim id stamped for this delivery.
func EventClaimID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	claimID, _ := ctx.Value(eventClaimContextKey{}).(string)
	return strings.TrimSpace(claimID)
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
