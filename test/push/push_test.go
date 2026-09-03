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
	"github.com/wecredit/communication-sdk/internal/channels/push/audit"
	"github.com/wecredit/communication-sdk/internal/channels/push/fcm"
	"github.com/wecredit/communication-sdk/internal/channels/push/ledger"
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

func TestDataOnlyFCMPayload(t *testing.T) {
	request, err := fcm.BuildDataOnlyRequest("secret-device-token", "Offer ready", "Open the app", sdkModels.CommApiRequestBody{
		EventId:           "event-1",
		UserId:            "user-1",
		NotificationEvent: "offer_view",
		DeepLink:          "zapcash://offers/1",
		NavigationData: map[string]string{
			"screen": "offer",
			"title":  "must-not-override",
		},
	})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if request.Message.Data["title"] != "Offer ready" {
		t.Fatalf("reserved title was overridden: %q", request.Message.Data["title"])
	}

	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(raw), `"notification"`) {
		t.Fatalf("payload contains forbidden notification block: %s", raw)
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

func TestAuditDispatcherIsNonBlockingWhenFull(t *testing.T) {
	dispatcher, err := audit.NewDispatcher(1, 1, func(audit.Job) error {
		select {}
	}, nil)
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	if !dispatcher.TrySubmit(audit.Job{Input: &audit.Input{EventID: "one"}}) {
		t.Fatal("first audit should be accepted")
	}

	deadline := time.Now().Add(time.Second)
	for dispatcher.TrySubmit(audit.Job{Input: &audit.Input{EventID: "fill"}}) && time.Now().Before(deadline) {
	}
	start := time.Now()
	if dispatcher.TrySubmit(audit.Job{Output: &audit.Output{EventID: "drop"}}) {
		t.Fatal("audit should be dropped when dispatcher is saturated")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("saturated audit submission blocked")
	}
}

func TestTokenFingerprintDoesNotContainToken(t *testing.T) {
	const token = "complete-secret-device-token"
	fingerprint, err := ledger.FingerprintToken(token)
	if err != nil {
		t.Fatalf("fingerprint token: %v", err)
	}
	if len(fingerprint) != 64 || strings.Contains(fingerprint, token) {
		t.Fatalf("unsafe token fingerprint %q", fingerprint)
	}
}

type fakeLedger struct {
	mu       sync.Mutex
	nextID   uint64
	attempts map[uint64]int
}

func (l *fakeLedger) CreateOrGetPending(_ context.Context, _ ledger.Identity, token string) (ledger.Dispatch, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextID++
	fingerprint, _ := ledger.FingerprintToken(token)
	return ledger.Dispatch{ID: l.nextID, Status: ledger.StatusPending, TokenFingerprint: fingerprint}, true, nil
}

func (l *fakeLedger) Claim(_ context.Context, id uint64) (ledger.Dispatch, bool, error) {
	now := time.Now()
	return ledger.Dispatch{ID: id, Status: ledger.StatusClaimed, ClaimedAt: &now, TokenFingerprint: strings.Repeat("a", 64)}, true, nil
}

func (l *fakeLedger) IsClaimCurrent(context.Context, uint64) (bool, time.Duration, error) {
	return true, time.Second, nil
}

func (l *fakeLedger) RecordAttempt(_ context.Context, id uint64, attempt int) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.attempts == nil {
		l.attempts = make(map[uint64]int)
	}
	l.attempts[id] = attempt
	return true, nil
}

func (l *fakeLedger) Finalize(context.Context, uint64, ledger.Status, int, string, string) (bool, error) {
	return true, nil
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

	store := &fakeLedger{}
	executor := &fakeExecutor{}
	service, err := push.NewService(store, executor)
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
	if len(executor.payloads) != 2 {
		t.Fatalf("provider payload count = %d, want 2", len(executor.payloads))
	}
	for _, payload := range executor.payloads {
		if payload.Message.Data["title"] != "Hi Ronit" || strings.Contains(payload.Message.Data["body"], "{{firstName}}") {
			t.Fatalf("template variables were not resolved: %+v", payload.Message.Data)
		}
	}
}

func TestAttemptObserverFailurePreventsProviderCall(t *testing.T) {
	sender := &sequenceSender{responses: []fcm.SendResponse{{HTTPStatus: http.StatusOK, MessageID: "unexpected"}}}
	executor, err := fcm.NewRetryExecutor(sender)
	if err != nil {
		t.Fatalf("new retry executor: %v", err)
	}
	_, err = executor.ExecuteWithObserver(context.Background(), "zapcash", fcm.SendRequest{}, nil,
		func(context.Context, int) error { return errors.New("ledger unavailable") })
	if err == nil {
		t.Fatal("expected observer failure")
	}
	if sender.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", sender.calls)
	}
}
