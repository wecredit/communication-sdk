package pinnacleIvr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	// ErrCode / ErrDesc are parsed Pinnacle fields (string JSON only).
	ErrCode string
	ErrDesc string
	// RawBody is the exact HTTP response body from Pinnacle (capped at read limit).
	RawBody string
	BodyLen int
	BodyFP  string
	// ApiURL / RequestJSON / ApiCurl are the outbound call with Password masked as "***".
	ApiURL      string
	RequestJSON string
	ApiCurl     string
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

	requestJSON, apiCurl := buildLoggedRequest(url, body)

	payload, err := json.Marshal(body)
	if err != nil {
		return CallResult{
			ApiURL:      url,
			RequestJSON: requestJSON,
			ApiCurl:     apiCurl,
		}, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, IvrHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return CallResult{
			ApiURL:      url,
			RequestJSON: requestJSON,
			ApiCurl:     apiCurl,
		}, err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := ivrHTTPClient.Do(req)
	if err != nil {
		return CallResult{
			ResponseMessage: capMessage(PrefixUnknown + " transport"),
			RedisCategory:   "ivr_outcome_unknown",
			ApiURL:          url,
			RequestJSON:     requestJSON,
			ApiCurl:         apiCurl,
		}, err
	}

	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return CallResult{
			HTTPStatus:      resp.StatusCode,
			ResponseMessage: capMessage(PrefixUnknown + " body"),
			RedisCategory:   "ivr_outcome_unknown",
			RawBody:         string(raw),
			BodyLen:         len(raw),
			BodyFP:          bodyFingerprint(raw),
			ApiURL:          url,
			RequestJSON:     requestJSON,
			ApiCurl:         apiCurl,
		}, readErr
	}

	result := parseCallResponse(resp.StatusCode, raw)
	result.ApiURL = url
	result.RequestJSON = requestJSON
	result.ApiCurl = apiCurl
	return result, nil
}

// buildLoggedRequest returns the JSON body and a copy-paste curl with Password masked.
func buildLoggedRequest(url string, body CallRequest) (requestJSON, apiCurl string) {
	logged := body
	if strings.TrimSpace(logged.Password) != "" {
		logged.Password = "***"
	}
	payload, err := json.Marshal(logged)
	if err != nil {
		return "", ""
	}
	requestJSON = string(payload)
	escaped := strings.ReplaceAll(requestJSON, "'", `'\''`)
	apiCurl = fmt.Sprintf("curl -sS -X POST '%s' -H 'Content-Type: application/json' -d '%s'", url, escaped)
	return requestJSON, apiCurl
}

func parseCallResponse(status int, raw []byte) CallResult {
	result := CallResult{
		HTTPStatus:      status,
		RedisCategory:   "ivr_outcome_unknown",
		ResponseMessage: PrefixUnknown,
		RawBody:         string(raw),
		BodyLen:         len(raw),
		BodyFP:          bodyFingerprint(raw),
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		result.ResponseMessage = capMessage(PrefixUnknown + " empty")
		return result
	}

	var payload struct {
		ErrCode json.RawMessage `json:"ERR_CODE"`
		ErrDesc json.RawMessage `json:"ERR_DESC"`
		CampgID json.RawMessage `json:"CAMPG_ID"`
		TransID json.RawMessage `json:"TRANS_ID"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		result.ResponseMessage = capMessage(PrefixUnknown + " malformed")
		return result
	}

	code, codeOK := jsonString(payload.ErrCode)
	if !codeOK {
		result.ResponseMessage = capMessage(PrefixUnknown + " err_code")
		return result
	}
	result.ErrCode = code

	if status < 200 || status >= 300 {
		if desc, ok := jsonString(payload.ErrDesc); ok {
			result.ErrDesc = desc
		}
		result.ResponseMessage = capMessage(PrefixUnknown + " http")
		return result
	}

	desc, _ := jsonString(payload.ErrDesc)
	result.ErrDesc = desc
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

func bodyFingerprint(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:12]
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
