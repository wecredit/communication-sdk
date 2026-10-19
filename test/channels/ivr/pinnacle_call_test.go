package ivr_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/ivr"
	pinnacleIvr "github.com/wecredit/communication-sdk/internal/channels/ivr/pinnacle"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/internal/redis"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/pkg/cache"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
	"gorm.io/gorm"
)

func TestValidateIVRMobileIsTenDigits(t *testing.T) {
	valid := sdkModels.CommApiRequestBody{Channel: variables.IVR, ProcessName: "collection", Mobile: "7014850582"}
	if ok, message := sdkHelper.ValidateCommRequest(valid); !ok {
		t.Fatalf("10 digit mobile rejected: %s", message)
	}
	valid.Mobile = "701485058"
	if ok, _ := sdkHelper.ValidateCommRequest(valid); ok {
		t.Fatal("short mobile was accepted")
	}
}

func TestShouldReclaimBlankZapCashClaimIncludesIVR(t *testing.T) {
	for _, channel := range []string{variables.IVR, variables.RCS, variables.WhatsApp, variables.SMS} {
		if !services.ShouldReclaimBlankZapCashClaim(channel, "", "", 2) {
			t.Fatalf("%s redelivery must reclaim a blank claim", channel)
		}
		if services.ShouldReclaimBlankZapCashClaim(channel, "", "ivr_attempted", 2) {
			t.Fatalf("%s must keep a stamped claim", channel)
		}
	}
}

func TestPinnacleIVROneCall(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		doErr      error
		wantSent   bool
		wantPrefix string
		wantTxn    string
		wantCamp   string
	}{
		{name: "accepted", status: 200, body: `{"ERR_CODE":"0","ERR_DESC":"Request processed Successfully.","CAMPG_ID":"651637","TRANS_ID":"10227391223"}`, wantSent: true, wantPrefix: pinnacleIvr.PrefixAccepted, wantTxn: "10227391223", wantCamp: "651637"},
		{name: "rejected", status: 200, body: `{"ERR_CODE":"5","ERR_DESC":"Vox file not found","CAMPG_ID":"-1","TRANS_ID":"-1"}`, wantPrefix: pinnacleIvr.PrefixRejected, wantTxn: "-1", wantCamp: "-1"},
		{name: "http 400", status: 400, body: `nope`, wantPrefix: pinnacleIvr.PrefixUnknown},
		{name: "http 500", status: 500, body: `{"ERR_CODE":1}`, wantPrefix: pinnacleIvr.PrefixUnknown},
		{name: "http 302", status: 302, body: `{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"1","TRANS_ID":"2"}`, wantPrefix: pinnacleIvr.PrefixUnknown},
		{name: "empty", status: 200, body: ``, wantPrefix: pinnacleIvr.PrefixUnknown},
		{name: "malformed", status: 200, body: `{`, wantPrefix: pinnacleIvr.PrefixUnknown},
		{name: "missing code", status: 200, body: `{"ERR_DESC":"ok","CAMPG_ID":"1","TRANS_ID":"2"}`, wantPrefix: pinnacleIvr.PrefixUnknown},
		{name: "numeric zero", status: 200, body: `{"ERR_CODE":0,"TRANS_ID":10227391223,"CAMPG_ID":"1234567890123456789012345678901"}`, wantPrefix: pinnacleIvr.PrefixUnknown},
		{name: "trailing", status: 200, body: `{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"1","TRANS_ID":"2"} junk`, wantPrefix: pinnacleIvr.PrefixUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			restore := pinnacleIvr.SetIvrRoundTripperForTest(roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if tc.doErr != nil {
					return nil, tc.doErr
				}
				return &http.Response{
					StatusCode: tc.status,
					Body:       io.NopCloser(strings.NewReader(tc.body)),
					Header:     make(http.Header),
				}, nil
			}))
			defer restore()
			result, err := pinnacleIvr.PostSingleCall(context.Background(), "http://103.132.146.183/OBD_REST_API/", pinnacleIvr.CallRequest{
				UserName: "user", Password: "secret-ivr-password", VoiceId: "57965", MSISDN: "7014850582",
				CustCallID: "WC-1", OBDType: "SINGLE_VOICE",
			})
			if tc.doErr != nil && err == nil {
				t.Fatal("expected transport error")
			}
			if calls.Load() != 1 {
				t.Fatalf("Do count = %d", calls.Load())
			}
			if result.IsSent != tc.wantSent {
				t.Fatalf("IsSent = %v", result.IsSent)
			}
			if !strings.HasPrefix(result.ResponseMessage, tc.wantPrefix) {
				t.Fatalf("message %q", result.ResponseMessage)
			}
			if result.TransactionID != tc.wantTxn || result.CampaignID != tc.wantCamp {
				t.Fatalf("ids txn=%q camp=%q", result.TransactionID, result.CampaignID)
			}
		})
	}
}

func TestPinnacleIVRTransportErrorIsOneCall(t *testing.T) {
	var calls atomic.Int32
	restore := pinnacleIvr.SetIvrRoundTripperForTest(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, context.DeadlineExceeded
	}))
	defer restore()
	result, err := pinnacleIvr.PostSingleCall(context.Background(), "http://103.132.146.183/OBD_REST_API", pinnacleIvr.CallRequest{OBDType: "SINGLE_VOICE"})
	if err == nil || calls.Load() != 1 || result.IsSent || result.RedisCategory != "ivr_outcome_unknown" {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, calls.Load())
	}
	if pinnacleIvr.IvrHTTPTimeout != 30*time.Second {
		t.Fatalf("timeout = %s", pinnacleIvr.IvrHTTPTimeout)
	}
}

func TestIVRAttemptDoesNotCallTwice(t *testing.T) {
	env := startIVR(t)
	body := `{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"651637","TRANS_ID":"10227391223"}`
	env.script(200, body, nil)
	first := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{"Mobile": int64(7014850582)})
	if env.calls.Load() != 1 || !first.DeleteMessage {
		t.Fatalf("first calls=%d delete=%v", env.calls.Load(), first.DeleteMessage)
	}
	second := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
	if env.calls.Load() != 1 || !second.DeleteMessage {
		t.Fatalf("redelivery calls=%d delete=%v", env.calls.Load(), second.DeleteMessage)
	}
	logs := env.logs.String()
	if strings.Contains(logs, "secret-ivr-password") || strings.Contains(logs, "OMNISTACK") {
		t.Fatalf("credentials leaked into logs: %s", logs)
	}
	if !strings.Contains(logs, "ivr_audit_enqueue commId=WC-ivr-1 table=IvrOutputTable") {
		t.Fatalf("missing audit enqueue log: %s", logs)
	}
}

func TestIVRAcceptedWithoutTransactionStoresOutcome(t *testing.T) {
	env := startIVR(t)
	env.script(200, `{"ERR_CODE":"0","ERR_DESC":"ok"}`, nil)
	got := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
	if env.calls.Load() != 1 || !got.DeleteMessage {
		t.Fatalf("calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
	}
	value, ok := env.claims.get("ivr-test", "event-1")
	if !ok || value != "ivr_accepted" {
		t.Fatalf("claim %q ok=%v", value, ok)
	}
}

func TestIVRActiveNonPinnacleVendorDoesNotCall(t *testing.T) {
	env := startIVR(t)
	cache.GetCache().Set(cache.VendorsData, map[string]map[string]interface{}{
		"Name:SINCH|Channel:IVR|Client:zapcash": {"Status": int64(1)},
	})
	cache.GetCache().Wait()
	req := env.request()
	req.Vendor = "SINCH"
	got := ivr.SendAttempt(context.Background(), req, map[string]interface{}{})
	if env.calls.Load() != 0 || !got.DeleteMessage || got.InactiveVendor {
		t.Fatalf("calls=%d delete=%v inactive=%v", env.calls.Load(), got.DeleteMessage, got.InactiveVendor)
	}
}

func TestIVRRejectedAndUnknownDoNotRetry(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		env := startIVR(t)
		env.script(200, `{"ERR_CODE":"6","ERR_DESC":"Incorrect OBD TYPE Code","CAMPG_ID":"-1","TRANS_ID":"-1"}`, nil)
		got := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
		if env.calls.Load() != 1 || !got.DeleteMessage {
			t.Fatalf("rejected calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
		}
		again := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
		if env.calls.Load() != 1 || !again.DeleteMessage {
			t.Fatalf("redelivery calls=%d", env.calls.Load())
		}
	})

	t.Run("unknown", func(t *testing.T) {
		env := startIVR(t)
		env.script(200, `not-json`, nil)
		got := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
		if env.calls.Load() != 1 || !got.DeleteMessage {
			t.Fatalf("unknown calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
		}
		value, ok := env.claims.get("ivr-test", "event-1")
		if !ok || value != "ivr_outcome_unknown" {
			t.Fatalf("claim %q ok=%v", value, ok)
		}
		again := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
		if env.calls.Load() != 1 || !again.DeleteMessage {
			t.Fatalf("redelivery calls=%d delete=%v", env.calls.Load(), again.DeleteMessage)
		}
	})
}

func TestIVRStampMissDoesNotCallOrDelete(t *testing.T) {
	env := startIVR(t)
	env.script(200, `{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"1","TRANS_ID":"2"}`, nil)
	env.claims.stampErr = errors.New("redis unavailable")
	got := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
	if env.calls.Load() != 0 || got.DeleteMessage {
		t.Fatalf("stamp error calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
	}
	if value, ok := env.claims.get("ivr-test", "event-1"); !ok || value != "" {
		t.Fatalf("claim changed to %q ok=%v", value, ok)
	}
}

func TestIVRAbsentClaimDoesNotCall(t *testing.T) {
	env := startIVR(t)
	env.script(200, `{"ERR_CODE":"0"}`, nil)
	req := env.request()
	req.EventId = "missing-field"
	got := ivr.SendAttempt(context.Background(), req, map[string]interface{}{})
	if env.calls.Load() != 0 || got.DeleteMessage {
		t.Fatalf("absent claim calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
	}
}

func TestIVRInvalidVoiceDoesNotCall(t *testing.T) {
	for _, voice := range []string{"", "abc", "65a"} {
		env := startIVR(t)
		env.voice = voice
		env.installTemplate()
		env.script(200, `{"ERR_CODE":"0","TRANS_ID":"1","CAMPG_ID":"1","ERR_DESC":"ok"}`, nil)
		got := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
		if env.calls.Load() != 0 || !got.DeleteMessage {
			t.Fatalf("voice %q calls=%d delete=%v", voice, env.calls.Load(), got.DeleteMessage)
		}
	}
}

func TestIVRRequestBody(t *testing.T) {
	env := startIVR(t)
	var raw []byte
	var method, url string
	env.round = func(r *http.Request) (*http.Response, error) {
		env.calls.Add(1)
		method = r.Method
		url = r.URL.String()
		raw, _ = io.ReadAll(r.Body)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"651637","TRANS_ID":"10227391223"}`)), Header: make(http.Header)}, nil
	}
	got := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
	if !got.DeleteMessage || env.calls.Load() != 1 || method != http.MethodPost {
		t.Fatalf("call method=%s calls=%d", method, env.calls.Load())
	}
	if !strings.Contains(url, "/OBD_REST_API/api/OBD_Rest/SINGLE_CALLWithCustCall_ID") {
		t.Fatalf("url %s", url)
	}
	var body map[string]string
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"OBD_TYPE", "VoiceId", "MSISDN", "CustCall_ID", "PARAM1", "PARAM2", "PARAM3", "PARAM4", "DTMF", "THX_VOX_ID"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing %s in %s", key, raw)
		}
	}
	if body["OBD_TYPE"] != "SINGLE_VOICE" || body["VoiceId"] != "57965" || body["PARAM1"] != "" || body["DTMF"] != "" || body["THX_VOX_ID"] != "" {
		t.Fatalf("body %s", raw)
	}
	if strings.Contains(env.logs.String(), "secret-ivr-password") || strings.Contains(env.logs.String(), "7014850582") {
		t.Fatal("secret or msisdn logged")
	}
	if body["UserName"] == "" || body["Password"] == "" {
		t.Fatalf("credentials missing from the request body: %s", raw)
	}
}

func TestIVRCancelledAfterStampDoesNotCall(t *testing.T) {
	env := startIVR(t)
	ctx, cancel := context.WithCancel(context.Background())
	restore := ivr.SetClaimHooksForTest(func(hash, field, marker string) (int, error) {
		result, err := env.claims.stamp(hash, field, marker)
		cancel()
		return result, err
	}, env.claims.outcome, env.claims.reclaim)
	t.Cleanup(restore)

	got := ivr.SendAttempt(ctx, env.request(), map[string]interface{}{})
	if env.calls.Load() != 0 || !got.DeleteMessage {
		t.Fatalf("cancelled calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
	}
	value, ok := env.claims.get("ivr-test", "event-1")
	if !ok || value != "ivr_outcome_unknown" {
		t.Fatalf("claim %q ok=%v", value, ok)
	}
	again := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
	if env.calls.Load() != 0 || !again.DeleteMessage {
		t.Fatalf("redelivery calls=%d delete=%v", env.calls.Load(), again.DeleteMessage)
	}
}

func TestIVRCancelledBeforeStampReleasesBlankClaim(t *testing.T) {
	env := startIVR(t)
	env.script(200, `{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"1","TRANS_ID":"2"}`, nil)
	req := env.request()
	req.EventId = "rate-limit-cancel"
	req.CommId = "WC-cancel"
	env.claims.seed("ivr-test", req.EventId, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := ivr.SendAttempt(ctx, req, map[string]interface{}{})
	if env.calls.Load() != 0 || got.DeleteMessage {
		t.Fatalf("cancelled calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
	}
	if _, ok := env.claims.get("ivr-test", req.EventId); ok {
		t.Fatal("blank claim was not released")
	}
	if !strings.Contains(env.logs.String(), "ivr_rate_limit_cancelled") {
		t.Fatalf("missing cancel log: %s", env.logs.String())
	}
}

func TestIvrOutcomeRejectsControlCharacter(t *testing.T) {
	err := redis.SetIvrClaimOutcome(nil, "hash", "field", "10227\n391", "")
	if err == nil || !strings.Contains(err.Error(), "ivr outcome value rejected") {
		t.Fatalf("newline transaction id: %v", err)
	}
	err = redis.SetIvrClaimOutcome(nil, "hash", "field", "10227391223", "")
	if err == nil || !strings.Contains(err.Error(), "redis client is nil") {
		t.Fatalf("plain transaction id: %v", err)
	}
}

func TestIVROverlappingDeliveriesCallOnce(t *testing.T) {
	env := startIVR(t)
	env.script(200, `{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"11","TRANS_ID":"22"}`, nil)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
		}()
	}
	wg.Wait()
	if env.calls.Load() != 1 {
		t.Fatalf("overlapping Do count = %d", env.calls.Load())
	}
}

func TestIVRAuditBusyStillDeletesAfterOneCall(t *testing.T) {
	env := startIVR(t)
	env.script(200, `{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"651637","TRANS_ID":"10227391223"}`, nil)
	previous := database.DBtechWrite
	database.DBtechWrite = &gorm.DB{}
	t.Cleanup(func() { database.DBtechWrite = previous })
	database.ConfigureAuditInsertSlots(1)
	release := make(chan struct{})
	if !database.EnqueueAuditInsert(func() { <-release }) {
		t.Fatal("slot was not taken")
	}
	defer close(release)
	got := ivr.SendAttempt(context.Background(), env.request(), map[string]interface{}{})
	if env.calls.Load() != 1 || !got.DeleteMessage {
		t.Fatalf("busy audit calls=%d delete=%v", env.calls.Load(), got.DeleteMessage)
	}
	if !strings.Contains(env.logs.String(), "audit_insert_failed") || !strings.Contains(env.logs.String(), "audit insert busy") {
		t.Fatalf("missing busy log: %s", env.logs.String())
	}
}

type ivrEnv struct {
	t       *testing.T
	claims  *claimMap
	calls   atomic.Int32
	logs    bytes.Buffer
	voice   string
	round   func(*http.Request) (*http.Response, error)
	restore func()
}

func startIVR(t *testing.T) *ivrEnv {
	t.Helper()
	cache.InitializeCache()
	previousConfig := config.Configs
	config.Configs.CommIdempotentKey = "ivr-test"
	config.Configs.PinnacleZapcashIvrApiUrl = "http://103.132.146.183/OBD_REST_API/"
	config.Configs.PinnacleZapcashIvrApiUsername = "OMNISTACK"
	config.Configs.PinnacleZapcashIvrApiPassword = "secret-ivr-password"
	config.Configs.SdkIvrInputTable = "IvrInputAuditTable"
	config.Configs.IvrOutputTable = "IvrOutputTable"
	previousLogger := utils.Logger.Writer()
	env := &ivrEnv{t: t, claims: newClaimMap(), voice: "57965"}
	utils.Logger.SetOutput(&env.logs)
	env.installTemplate()
	cache.GetCache().Set(cache.VendorsData, map[string]map[string]interface{}{
		"Name:PINNACLE|Channel:IVR|Client:zapcash": {"Status": int64(1)},
	})
	cache.GetCache().Set(cache.ClientsData, map[string]map[string]interface{}{
		"Name:zapcash|Channel:IVR": {"ShouldHitVendor": variables.Active},
	})
	cache.GetCache().Wait()
	env.claims.seed("ivr-test", "event-1", "")
	restoreClaims := ivr.SetClaimHooksForTest(env.claims.stamp, env.claims.outcome, env.claims.reclaim)
	env.restore = pinnacleIvr.SetIvrRoundTripperForTest(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if env.round != nil {
			return env.round(r)
		}
		env.calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ERR_CODE":"0","ERR_DESC":"ok","CAMPG_ID":"1","TRANS_ID":"2"}`)), Header: make(http.Header)}, nil
	}))
	t.Cleanup(func() {
		env.restore()
		restoreClaims()
		utils.Logger.SetOutput(previousLogger)
		config.Configs = previousConfig
	})
	return env
}

type claimMap struct {
	mu       sync.Mutex
	fields   map[string]string
	present  map[string]bool
	stampErr error
}

func newClaimMap() *claimMap {
	return &claimMap{fields: map[string]string{}, present: map[string]bool{}}
}

func claimKey(hash, field string) string { return hash + "\x00" + field }

func (c *claimMap) seed(hash, field, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := claimKey(hash, field)
	c.fields[key] = value
	c.present[key] = true
}

func (c *claimMap) get(hash, field string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := claimKey(hash, field)
	value, ok := c.fields[key]
	return value, ok && c.present[key]
}

func (c *claimMap) stamp(hash, field, marker string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stampErr != nil {
		return 0, c.stampErr
	}
	key := claimKey(hash, field)
	if !c.present[key] {
		return -1, nil
	}
	if c.fields[key] != "" {
		return 0, nil
	}
	c.fields[key] = marker
	return 1, nil
}

func (c *claimMap) outcome(hash, field, transactionID, category string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := claimKey(hash, field)
	if !c.present[key] || c.fields[key] != "ivr_attempted" {
		return errors.New("ivr claim is not attempted")
	}
	if transactionID != "" {
		c.fields[key] = transactionID
		return nil
	}
	if category == "" {
		return errors.New("ivr outcome empty")
	}
	c.fields[key] = category
	return nil
}

func (c *claimMap) reclaim(hash, field string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := claimKey(hash, field)
	if !c.present[key] || c.fields[key] != "" {
		return false, nil
	}
	delete(c.fields, key)
	delete(c.present, key)
	return true, nil
}

func (e *ivrEnv) script(status int, body string, doErr error) {
	e.round = func(r *http.Request) (*http.Response, error) {
		e.calls.Add(1)
		if doErr != nil {
			return nil, doErr
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}
}

func (e *ivrEnv) installTemplate() {
	stage := 13.01
	snapshot, err := cache.BuildTemplateSnapshot([]apiModels.Templatedetails{{
		Id: 1, Client: "zapcash", Channel: "IVR", Process: "ZAPCASH", Stage: &stage,
		Vendor: "PINNACLE", TemplateName: "ZapDueDateeng", VoiceId: e.voice, IsActive: true,
	}})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := cache.InstallTemplateSnapshot(snapshot); err != nil {
		e.t.Fatal(err)
	}
}

func (e *ivrEnv) request() *sdkModels.CommApiRequestBody {
	return &sdkModels.CommApiRequestBody{
		CommId: "WC-ivr-1", EventId: "event-1", Mobile: "7014850582", Channel: variables.IVR,
		ProcessName: "ZAPCASH", Stage: 13.01, Client: "zapcash", Vendor: variables.PINNACLE,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
