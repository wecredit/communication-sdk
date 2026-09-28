package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// RetryApiCall handles retries for an API call using the shared HTTP client/transport.
// Non-2xx responses still return (result, nil) with ApistatusCode set — many callers
// inspect status themselves. Use ErrIfHTTPNotOK when a call site needs hard failure.
func RetryApiCall(
	method, apiURL string,
	headers map[string]string,
	username, password string,
	data interface{},
	reqType int,
	retryMax int,
	retryWaitMin, retryWaitMax time.Duration,
) (map[string]interface{}, error) {
	statusCode, body, err := doAPIRequest(method, apiURL, headers, username, password, data, reqType, retryMax, retryWaitMin, retryWaitMax)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}

	Info(fmt.Sprintf("API_RESPONSE status=%d keys=%d", statusCode, len(result)))
	result["ApistatusCode"] = statusCode
	return result, nil
}

// ApiHit makes an API call using the shared HTTP client (no per-call goroutine/client).
func ApiHit(method, apiURL string, headers map[string]string, username, password string, data interface{}, reqType int) (map[string]interface{}, error) {
	return RetryApiCall(method, apiURL, headers, username, password, data, reqType, 0, 0, 0)
}

// HTTPStatusFromResponse reads ApistatusCode set by RetryApiCall/ApiHit.
func HTTPStatusFromResponse(result map[string]interface{}) int {
	if result == nil {
		return 0
	}
	switch v := result["ApistatusCode"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

// ErrIfHTTPNotOK returns an error when ApistatusCode is missing or outside 2xx.
func ErrIfHTTPNotOK(result map[string]interface{}) error {
	code := HTTPStatusFromResponse(result)
	if code < 200 || code >= 300 {
		return fmt.Errorf("HTTP status %d", code)
	}
	return nil
}

// ApiHitJSON POSTs/GETs like ApiHit but unmarshals the body into dest (typed struct).
// Returns HTTP status and raw body for audit/logging. Dest may be nil to skip unmarshal.
func ApiHitJSON(method, apiURL string, headers map[string]string, username, password string, data interface{}, reqType int, dest interface{}) (statusCode int, rawBody string, err error) {
	statusCode, body, err := doAPIRequest(method, apiURL, headers, username, password, data, reqType, 0, 0, 0)
	if err != nil {
		return 0, "", err
	}

	rawBody = string(body)
	Info(fmt.Sprintf("API_RESPONSE status=%d bytes=%d", statusCode, len(body)))
	if dest != nil {
		if err := json.Unmarshal(body, dest); err != nil {
			return statusCode, rawBody, fmt.Errorf("error unmarshalling response: %v", err)
		}
	}

	return statusCode, rawBody, nil
}

func doAPIRequest(
	method, apiURL string,
	headers map[string]string,
	username, password string,
	data interface{},
	reqType int,
	retryMax int,
	retryWaitMin, retryWaitMax time.Duration,
) (statusCode int, body []byte, err error) {
	client := SharedHTTPClient(retryMax, retryWaitMin, retryWaitMax)

	var bodyReader io.Reader
	if method != "GET" && method != "get" {
		switch reqType {
		case variables.ContentTypeFormEncoded:
			formData, ok := data.(map[string]string)
			if !ok {
				return 0, nil, fmt.Errorf("data must be of type map[string]string for form encoding")
			}
			formValues := url.Values{}
			for key, value := range formData {
				formValues.Set(key, value)
			}
			bodyReader = bytes.NewBufferString(formValues.Encode())
		case variables.ContentTypeText:
			rawData, ok := data.(string)
			if !ok {
				return 0, nil, fmt.Errorf("data must be a string for Content-Type text/plain")
			}
			bodyReader = bytes.NewBufferString(rawData)
		default:
			if data == nil {
				bodyReader = bytes.NewBuffer(nil)
			} else {
				jsonData, err := json.Marshal(data)
				if err != nil {
					return 0, nil, fmt.Errorf("error marshalling data: %v", err)
				}
				bodyReader = bytes.NewBuffer(jsonData)
			}
		}
	}

	var req *retryablehttp.Request
	switch method {
	case "POST", "post":
		req, err = retryablehttp.NewRequest(http.MethodPost, apiURL, bodyReader)
	case "PUT", "put":
		req, err = retryablehttp.NewRequest(http.MethodPut, apiURL, bodyReader)
	case "GET", "get":
		req, err = retryablehttp.NewRequest(http.MethodGet, apiURL, nil)
	default:
		return 0, nil, fmt.Errorf("invalid HTTP method: %s", method)
	}
	if err != nil {
		return 0, nil, fmt.Errorf("error creating request: %v", err)
	}

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if reqType == variables.ContentTypeFormEncoded {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("error reading response body: %v", err)
	}
	return resp.StatusCode, body, nil
}
