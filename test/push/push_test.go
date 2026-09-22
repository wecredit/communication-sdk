package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/channels/push"
	"github.com/wecredit/communication-sdk/internal/channels/push/fcm"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestPushRequestValidation(t *testing.T) {
	valid := sdkModels.CommApiRequestBody{
		Channel: "push", ProcessName: "offer", Client: "zapcash",
		EventId: "event-1", DeviceTokens: []string{"token-1"},
	}
	if ok, message := sdkHelper.ValidateCommRequest(valid); !ok {
		t.Fatalf("valid PUSH request rejected: %s", message)
	}

	valid.EventId = ""
	if ok, _ := sdkHelper.ValidateCommRequest(valid); ok {
		t.Fatal("PUSH request without eventId was accepted")
	}
	valid.EventId = "event-1"
	valid.DeviceTokens = []string{"  "}
	if ok, _ := sdkHelper.ValidateCommRequest(valid); ok {
		t.Fatal("PUSH request without a usable device token was accepted")
	}
}

func TestFCMPayloadIncludesNotificationAndData(t *testing.T) {
	request, err := fcm.BuildSendRequest("secret-device-token", "Offer ready", "Open the app", sdkModels.CommApiRequestBody{
		EventId:           "event-1",
		UserId:            "user-1",
		NotificationEvent: "offer_view",
		DeepLink:          "zapcash://offers/1",
		NavigationData: map[string]string{
			"screen":             "offer",
			"title":              "must-not-override",
			"from":               "reserved",
			"google.c.sender.id": "reserved",
		},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if request.Message.Data["title"] != "Offer ready" {
		t.Fatalf("reserved title was overridden: %q", request.Message.Data["title"])
	}
	if request.Message.Notification == nil || request.Message.Notification.Title != "Offer ready" || request.Message.Notification.Body != "Open the app" {
		t.Fatalf("notification = %+v, want title/body matching template", request.Message.Notification)
	}
	if request.Message.Android == nil || request.Message.Android.Priority != "HIGH" {
		t.Fatalf("android = %+v, want priority HIGH", request.Message.Android)
	}
	if _, ok := request.Message.Data["from"]; ok {
		t.Fatal("reserved FCM data key from must be removed")
	}
	if _, ok := request.Message.Data["google.c.sender.id"]; ok {
		t.Fatal("reserved FCM data key google.* must be removed")
	}

	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if !strings.Contains(string(raw), `"notification"`) {
		t.Fatalf("payload missing notification block: %s", raw)
	}
}

func TestFCMOutcomeClassification(t *testing.T) {
	tests := []struct {
		name     string
		response fcm.SendResponse
		want     fcm.Outcome
	}{
		{name: "unregistered is permanent", response: fcm.SendResponse{HTTPStatus: 404, ErrorCode: "UNREGISTERED"}, want: fcm.OutcomeFailedFinal},
		{name: "throttling retries", response: fcm.SendResponse{HTTPStatus: http.StatusTooManyRequests}, want: fcm.OutcomeRetryable},
		{name: "service unavailable retries", response: fcm.SendResponse{HTTPStatus: http.StatusServiceUnavailable}, want: fcm.OutcomeRetryable},
		{name: "accepted", response: fcm.SendResponse{HTTPStatus: http.StatusOK, MessageID: "projects/p/messages/1"}, want: fcm.OutcomeSubmitted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fcm.Classify(test.response, nil).Outcome; got != test.want {
				t.Fatalf("outcome = %q, want %q", got, test.want)
			}
		})
	}
}

type sequenceSender struct {
	mu        sync.Mutex
	responses []fcm.SendResponse
	calls     int
}

func (s *sequenceSender) Send(context.Context, string, fcm.SendRequest) (fcm.SendResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	response := s.responses[s.calls]
	s.calls++
	return response, nil
}

func TestRetryExecutorMakesExactlyTwoTotalAttempts(t *testing.T) {
	sender := &sequenceSender{responses: []fcm.SendResponse{
		{HTTPStatus: http.StatusServiceUnavailable},
		{HTTPStatus: http.StatusOK, MessageID: "projects/p/messages/1"},
	}}
	executor, err := fcm.NewRetryExecutor(sender)
	if err != nil {
		t.Fatalf("new retry executor: %v", err)
	}
	observed := make([]int, 0, 2)
	result, err := executor.ExecuteWithObserver(context.Background(), "zapcash", fcm.SendRequest{}, nil,
		func(_ context.Context, attempt int) error {
			observed = append(observed, attempt)
			return nil
		})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sender.calls != 2 || result.AttemptCount != 2 || result.Outcome != fcm.OutcomeSubmitted {
		t.Fatalf("calls=%d result=%+v, want two attempts and submitted", sender.calls, result)
	}
	if len(observed) != 2 || observed[0] != 1 || observed[1] != 2 {
		t.Fatalf("observed attempts = %v, want [1 2]", observed)
	}
}

func TestTokenFingerprintDoesNotContainToken(t *testing.T) {
	const token = "complete-secret-device-token"
	fingerprint, err := push.FingerprintToken(token)
	if err != nil {
		t.Fatalf("fingerprint token: %v", err)
	}
	if len(fingerprint) != 64 || strings.Contains(fingerprint, token) {
		t.Fatalf("unsafe token fingerprint %q", fingerprint)
	}
}

func TestTokenRedisFieldUsesEventIdBase(t *testing.T) {
	fp, err := push.FingerprintToken("token-a")
	if err != nil {
		t.Fatal(err)
	}
	field := push.TokenRedisField(sdkModels.CommApiRequestBody{
		EventId: "event-hash-1",
		CommId:  "WC-ZAPCASH-should-not-appear",
		Mobile:  "9876543210",
		Channel: "PUSH",
		Stage:   1,
	}, fp)
	if !strings.HasPrefix(field, "event-hash-1:") {
		t.Fatalf("field = %q, want EventId base", field)
	}
	if strings.Contains(field, "WC-ZAPCASH") {
		t.Fatalf("CommId leaked into Redis field: %q", field)
	}
	if !strings.HasSuffix(field, ":"+fp) {
		t.Fatalf("field missing fingerprint suffix: %q", field)
	}
}

// fakeClaims is an in-memory tokenClaimStore for unit tests.
type fakeClaims struct {
	mu        sync.Mutex
	fields    map[string]string // "" blank, "txn:..." or "err:..."
	claimedAt map[string]time.Time
}

func newFakeClaims() *fakeClaims {
	return &fakeClaims{fields: make(map[string]string), claimedAt: make(map[string]time.Time)}
}

func (c *fakeClaims) Get(field string) (bool, string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.fields[field]
	if !ok {
		return false, "", "", nil
	}
	if strings.HasPrefix(val, "txn:") {
		return true, strings.TrimPrefix(val, "txn:"), "", nil
	}
	if strings.HasPrefix(val, "err:") {
		return true, "", strings.TrimPrefix(val, "err:"), nil
	}
	return true, "", "", nil // blank claim
}

func (c *fakeClaims) Claim(field string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.fields[field]; exists {
		return errors.New("key already exists in redis")
	}
	c.fields[field] = ""
	c.claimedAt[field] = time.Now().UTC()
	return nil
}

func (c *fakeClaims) ClaimedAt(field string) (time.Time, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	claimedAt, ok := c.claimedAt[field]
	return claimedAt, ok, nil
}

func (c *fakeClaims) ReclaimExpired(field string, cutoff time.Time) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.fields[field]
	claimedAt, hasTimestamp := c.claimedAt[field]
	if !ok || val != "" || !hasTimestamp || claimedAt.After(cutoff) {
		return false, nil
	}
	delete(c.fields, field)
	delete(c.claimedAt, field)
	return true, nil
}

func (c *fakeClaims) SetTransactionID(field, transactionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fields[field] = "txn:" + transactionID
	return nil
}

func (c *fakeClaims) SetErrorMessage(field, errorMessage string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fields[field] = "err:" + errorMessage
	return nil
}

type fakeExecutor struct {
	mu       sync.Mutex
	payloads []fcm.SendRequest
}

func (e *fakeExecutor) ExecuteWithObserver(
	ctx context.Context,
	_ string,
	payload fcm.SendRequest,
	_ fcm.RetryGuard,
	observer fcm.AttemptObserver,
) (fcm.ExecutionResult, error) {
	if err := observer(ctx, 1); err != nil {
		return fcm.ExecutionResult{}, err
	}
	e.mu.Lock()
	e.payloads = append(e.payloads, payload)
	e.mu.Unlock()
	return fcm.ExecutionResult{Outcome: fcm.OutcomeSubmitted, MessageID: "message-1", AttemptCount: 1}, nil
}

func TestPushServiceDeduplicatesTokensAndResolvesTemplate(t *testing.T) {
	cache.InitializeCache()
	seedZapCashPushShouldHitVendor(t, true)
	stage := 1.0
	snapshot, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{{
		Id: 1, Client: "zapcash", Channel: "PUSH", Process: "OFFER", Stage: &stage,
		Vendor: "FCM", TemplateName: "offer-1", TemplateHeader: "Hi {{firstName}}",
		TemplateText: "Your offer is ready, {{firstName}}", IsActive: true,
	}})
	if err != nil {
		t.Fatalf("build template snapshot: %v", err)
	}
	if err := cache.InstallTemplateSnapshot(snapshot); err != nil {
		t.Fatalf("install template snapshot: %v", err)
	}

	claims := newFakeClaims()
	executor := &fakeExecutor{}
	service, err := push.NewService(claims, executor)
	if err != nil {
		t.Fatalf("new PUSH service: %v", err)
	}
	result, err := service.Send(context.Background(), sdkModels.CommApiRequestBody{
		CommId: "comm-1", EventId: "event-1", Client: "zapcash", Channel: "PUSH",
		ProcessName: "OFFER", Stage: 1, CustomerName: "Ronit",
		DeviceTokens: []string{"token-a", "token-a", "token-b"},
	})
	if err != nil {
		t.Fatalf("send PUSH: %v", err)
	}
	if !result.AckSQS || result.Submitted != 2 {
		t.Fatalf("result = %+v, want two submitted unique tokens", result)
	}
	if result.InputAudit == nil || result.InputAudit["EventId"] != "event-1" {
		t.Fatalf("InputAudit = %#v, want EventId event-1 for consumer InsertData", result.InputAudit)
	}
	if len(result.OutputAudits) != 2 {
		t.Fatalf("OutputAudits = %d, want 2 for consumer InsertData", len(result.OutputAudits))
	}
	if len(executor.payloads) != 2 {
		t.Fatalf("provider payload count = %d, want 2", len(executor.payloads))
	}
	for _, payload := range executor.payloads {
		if payload.Message.Data["title"] != "Hi Ronit" || strings.Contains(payload.Message.Data["body"], "{{firstName}}") {
			t.Fatalf("template variables were not resolved: %+v", payload.Message.Data)
		}
	}

	// Replay same EventId + tokens → Redis skip, no more FCM.
	executor2 := &fakeExecutor{}
	service2, err := push.NewService(claims, executor2)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service2.Send(context.Background(), sdkModels.CommApiRequestBody{
		CommId: "comm-2", EventId: "event-1", Client: "zapcash", Channel: "PUSH",
		ProcessName: "OFFER", Stage: 1, CustomerName: "Ronit",
		DeviceTokens: []string{"token-a", "token-b"},
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Skipped != 2 || len(executor2.payloads) != 0 {
		t.Fatalf("replay = %+v payloads=%d, want skip both without FCM", replay, len(executor2.payloads))
	}
	if len(replay.OutputAudits) != 2 {
		t.Fatalf("replay output audits = %d, want terminal replay for both tokens", len(replay.OutputAudits))
	}
}

func TestPushShouldHitVendorOffSkipsFCM(t *testing.T) {
	cache.InitializeCache()
	seedZapCashPushShouldHitVendor(t, false)

	claims := newFakeClaims()
	executor := &fakeExecutor{}
	service, err := push.NewService(claims, executor)
	if err != nil {
		t.Fatalf("new PUSH service: %v", err)
	}
	result, err := service.Send(context.Background(), sdkModels.CommApiRequestBody{
		CommId: "comm-1", EventId: "event-1", Client: "zapcash", Channel: "PUSH",
		ProcessName: "OFFER", Stage: 1,
		DeviceTokens: []string{"token-a", "token-b"},
	})
	if err != nil {
		t.Fatalf("send PUSH: %v", err)
	}
	if !result.AckSQS || !result.Processed || result.Skipped != 2 {
		t.Fatalf("result = %+v, want terminal skip of 2 tokens", result)
	}
	if len(executor.payloads) != 0 {
		t.Fatalf("FCM was called despite ShouldHitVendor off: %d payloads", len(executor.payloads))
	}
}

func TestPushDoesNotReclaimInFlightEventClaim(t *testing.T) {
	cache.InitializeCache()
	seedZapCashPushShouldHitVendor(t, true)

	claims := newFakeClaims()
	fingerprint, err := push.FingerprintToken("token-a")
	if err != nil {
		t.Fatalf("fingerprint token: %v", err)
	}
	field := push.TokenRedisField(sdkModels.CommApiRequestBody{
		EventId: "event-1", Client: "zapcash", Channel: "PUSH", ProcessName: "OFFER", Stage: 1,
	}, fingerprint)
	if err := claims.Claim(field); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	executor := &fakeExecutor{}
	service, err := push.NewService(claims, executor)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Send(context.Background(), sdkModels.CommApiRequestBody{
		EventId: "event-1", Client: "zapcash", Channel: "PUSH", ProcessName: "OFFER", Stage: 1,
		DeviceTokens: []string{"token-a"},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.Skipped != 1 || len(executor.payloads) != 0 {
		t.Fatalf("result=%+v payloads=%d, want skip without FCM", result, len(executor.payloads))
	}
}

func TestPushShouldHitVendorOffDoesNotAckWhenClaimFails(t *testing.T) {
	cache.InitializeCache()
	seedZapCashPushShouldHitVendor(t, false)

	service, err := push.NewService(&failingClaimStore{err: errors.New("redis unavailable")}, &fakeExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Send(context.Background(), sdkModels.CommApiRequestBody{
		CommId: "comm-1", EventId: "event-1", Client: "zapcash", Channel: "PUSH",
		ProcessName: "OFFER", Stage: 1, DeviceTokens: []string{"token-a"},
	})
	if err == nil || result.AckSQS || result.Processed {
		t.Fatalf("result=%+v err=%v, want retryable Redis claim failure", result, err)
	}
}

func TestPushServiceReplacesKnownTemplateVariables(t *testing.T) {
	cache.InitializeCache()
	seedZapCashPushShouldHitVendor(t, true)
	stage := 2.0
	snapshot, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{{
		Id: 2, Client: "zapcash", Channel: "PUSH", Process: "OFFER", Stage: &stage,
		Vendor: "FCM", TemplateName: "offer-2", TemplateHeader: "Hi {{CustomerName}}",
		TemplateText: "Loan {{LoanId}} for {{EmiAmount}}: {{PaymentLink}}", IsActive: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.InstallTemplateSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}

	executor := &fakeExecutor{}
	service, err := push.NewService(newFakeClaims(), executor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Send(context.Background(), sdkModels.CommApiRequestBody{
		CommId: "comm-2", EventId: "event-2", Client: "zapcash", Channel: "PUSH", ProcessName: "OFFER", Stage: 2,
		CustomerName: "Ronit", LoanId: "loan-1", EmiAmount: "25000", PaymentLink: "https://pay.example", DeviceTokens: []string{"token-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := executor.payloads[0]
	if payload.Message.Notification.Title != "Hi Ronit" || payload.Message.Notification.Body != "Loan loan-1 for 25000: https://pay.example" {
		t.Fatalf("template variables not replaced: %+v", payload.Message.Notification)
	}
}

func TestPushClaimRedisErrorDoesNotSkip(t *testing.T) {
	cache.InitializeCache()
	seedZapCashPushShouldHitVendor(t, true)
	stage := 1.0
	snapshot, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{{
		Id: 1, Client: "zapcash", Channel: "PUSH", Process: "OFFER", Stage: &stage,
		Vendor: "FCM", TemplateName: "offer-1", TemplateHeader: "Hi",
		TemplateText: "Body", IsActive: true,
	}})
	if err != nil {
		t.Fatalf("build template snapshot: %v", err)
	}
	if err := cache.InstallTemplateSnapshot(snapshot); err != nil {
		t.Fatalf("install template snapshot: %v", err)
	}

	claims := &failingClaimStore{err: errors.New("redis connection refused")}
	executor := &fakeExecutor{}
	service, err := push.NewService(claims, executor)
	if err != nil {
		t.Fatalf("new PUSH service: %v", err)
	}
	result, err := service.Send(context.Background(), sdkModels.CommApiRequestBody{
		CommId: "comm-1", EventId: "event-1", Client: "zapcash", Channel: "PUSH",
		ProcessName: "OFFER", Stage: 1,
		DeviceTokens: []string{"token-a"},
	})
	if err == nil {
		t.Fatal("expected claim redis error to surface")
	}
	if result.AckSQS || result.Skipped != 0 || len(executor.payloads) != 0 {
		t.Fatalf("result = %+v payloads=%d, want no ack/skip and no FCM", result, len(executor.payloads))
	}
}

// failingClaimStore returns a hard Redis error on Claim (not "already exists").
type failingClaimStore struct {
	err error
}

func (c *failingClaimStore) Get(string) (bool, string, string, error) {
	return false, "", "", nil
}
func (c *failingClaimStore) ClaimedAt(string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (c *failingClaimStore) Claim(string) error { return c.err }
func (c *failingClaimStore) ReclaimExpired(string, time.Time) (bool, error) {
	return false, nil
}
func (c *failingClaimStore) SetTransactionID(string, string) error { return nil }
func (c *failingClaimStore) SetErrorMessage(string, string) error  { return nil }

func seedZapCashPushShouldHitVendor(t *testing.T, on bool) {
	t.Helper()
	var status int64
	if on {
		status = 1
	}

	c := cache.GetCache()
	if c == nil {
		t.Fatal("cache not initialized")
	}
	ok := c.Set(cache.ClientsData, map[string]map[string]interface{}{
		"Name:zapcash|Channel:PUSH": {
			"Name":            "zapcash",
			"Channel":         "PUSH",
			"ShouldHitVendor": status,
		},
	})
	if !ok {
		t.Fatal("failed to seed ClientsData cache")
	}
	c.Wait()
}

func TestRetryGuardCancelsStaleSecondAttempt(t *testing.T) {
	sender := &sequenceSender{responses: []fcm.SendResponse{
		{HTTPStatus: http.StatusServiceUnavailable},
		{HTTPStatus: http.StatusOK, MessageID: "should-not-send"},
	}}
	executor, err := fcm.NewRetryExecutor(sender)
	if err != nil {
		t.Fatalf("new retry executor: %v", err)
	}
	result, err := executor.ExecuteWithObserver(context.Background(), "zapcash", fcm.SendRequest{},
		func(context.Context) (bool, error) { return false, nil },
		nil,
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sender.calls != 1 || result.Outcome != fcm.OutcomeCancelledStale || result.Code != "STALE_BEFORE_RETRY" {
		t.Fatalf("calls=%d result=%+v, want one attempt then cancelled_stale", sender.calls, result)
	}
}

func TestAttemptObserverFailurePreventsProviderCall(t *testing.T) {
	sender := &sequenceSender{responses: []fcm.SendResponse{{HTTPStatus: http.StatusOK, MessageID: "unexpected"}}}
	executor, err := fcm.NewRetryExecutor(sender)
	if err != nil {
		t.Fatalf("new retry executor: %v", err)
	}
	_, err = executor.ExecuteWithObserver(context.Background(), "zapcash", fcm.SendRequest{}, nil,
		func(context.Context, int) error { return errors.New("claim store unavailable") })
	if err == nil {
		t.Fatal("expected observer failure")
	}
	if sender.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", sender.calls)
	}
}
