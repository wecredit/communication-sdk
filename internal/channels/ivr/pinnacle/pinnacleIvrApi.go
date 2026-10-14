package pinnacleIvr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	IvrHTTPTimeout       = 30 * time.Second
	ivrDialTimeout       = 10 * time.Second
	singleCallPath       = "/api/OBD_Rest/SINGLE_CALLWithCustCall_ID"
	maxResponseMessage   = 1000
	maxCampaignIDLength  = 30
	maxTransactionLength = 255
)

const (
	PrefixAccepted = "provider_accepted:"
	PrefixRejected = "provider_rejected:"
	PrefixUnknown  = "provider_outcome_unknown:"
	PrefixNotSent  = "request_not_sent:"
)

// CallRequest is the one Pinnacle body. Username and password stay in the JSON
// body because that is the request that returned ERR_CODE "0".
type CallRequest struct {
	UserName   string `json:"UserName"`
	Password   string `json:"Password"`
	VoiceId    string `json:"VoiceId"`
	MSISDN     string `json:"MSISDN"`
	CustCallID string `json:"CustCall_ID"`
	PARAM1     string `json:"PARAM1"`
	PARAM2     string `json:"PARAM2"`
	PARAM3     string `json:"PARAM3"`
	PARAM4     string `json:"PARAM4"`
	OBDType    string `json:"OBD_TYPE"`
	DTMF       string `json:"DTMF"`
	THXVoxID   string `json:"THX_VOX_ID"`
}

type CallResult struct {
	IsSent          bool
	ResponseMessage string
	CampaignID      string
	TransactionID   string
	RedisCategory   string
	HTTPStatus      int
}

var ivrHTTPClient = newIvrHTTPClient()

func newIvrHTTPClient() *http.Client {
	return &http.Client{
		Timeout: IvrHTTPTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: ivrDialTimeout}).DialContext,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// SetIvrRoundTripperForTest replaces the transport and returns a restore function.
func SetIvrRoundTripperForTest(rt http.RoundTripper) func() {
	previous := ivrHTTPClient.Transport
	ivrHTTPClient.Transport = rt
	return func() { ivrHTTPClient.Transport = previous }
}

func SingleCallURL(base string) (string, error) {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" || strings.Contains(trimmed, "@") {
		return "", fmt.Errorf("ivr api url is not set")
	}

	return strings.TrimRight(trimmed, "/") + singleCallPath, nil
}

// PostSingleCall performs one HTTP POST. It does not retry.
func PostSingleCall(ctx context.Context, baseURL string, body CallRequest) (CallResult, error) {
	if err := ctx.Err(); err != nil {
		return CallResult{}, err
	}

	url, err := SingleCallURL(baseURL)
	if err != nil {
		return CallResult{}, err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return CallResult{}, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, IvrHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return CallResult{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := ivrHTTPClient.Do(req)
	if err != nil {
		return CallResult{
			ResponseMessage: capMessage(PrefixUnknown + " transport"),
			RedisCategory:   "ivr_outcome_unknown",
		}, err
	}

	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return CallResult{
			HTTPStatus:      resp.StatusCode,
			ResponseMessage: capMessage(PrefixUnknown + " body"),
			RedisCategory:   "ivr_outcome_unknown",
		}, readErr
	}

	return parseCallResponse(resp.StatusCode, raw), nil
}

func parseCallResponse(status int, raw []byte) CallResult {
	result := CallResult{HTTPStatus: status, RedisCategory: "ivr_outcome_unknown", ResponseMessage: PrefixUnknown}
	if len(bytes.TrimSpace(raw)) == 0 {
		result.ResponseMessage = capMessage(PrefixUnknown + " empty")
		return result
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload struct {
		ErrCode json.RawMessage `json:"ERR_CODE"`
		ErrDesc json.RawMessage `json:"ERR_DESC"`
		CampgID json.RawMessage `json:"CAMPG_ID"`
		TransID json.RawMessage `json:"TRANS_ID"`
	}

	if err := decoder.Decode(&payload); err != nil {
		result.ResponseMessage = capMessage(PrefixUnknown + " malformed")
		return result
	}

	code, codeOK := jsonString(payload.ErrCode)
	if !codeOK {
		result.ResponseMessage = capMessage(PrefixUnknown + " err_code")
		return result
	}

	if status < 200 || status >= 300 {
		result.ResponseMessage = capMessage(PrefixUnknown + " http")
		return result
	}

	desc, _ := jsonString(payload.ErrDesc)
	if camp, ok := jsonString(payload.CampgID); ok && len(camp) >= 1 && len(camp) <= maxCampaignIDLength {
		result.CampaignID = camp
	}

	if txn, ok := jsonString(payload.TransID); ok && len(txn) >= 1 && len(txn) <= maxTransactionLength {
		result.TransactionID = txn
	}

	if code == "0" {
		result.IsSent = true
		result.RedisCategory = ""
		result.ResponseMessage = capMessage(joinPrefix(PrefixAccepted, desc))
		return result
	}

	result.RedisCategory = "ivr_provider_rejected"
	result.ResponseMessage = capMessage(joinPrefix(PrefixRejected, desc))
	return result
}

func jsonString(raw json.RawMessage) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}

	return value, true
}

func joinPrefix(prefix, desc string) string {
	if desc == "" {
		return prefix
	}

	return prefix + " " + desc
}

func capMessage(message string) string {
	if len(message) <= maxResponseMessage {
		return message
	}

	return message[:maxResponseMessage]
}
