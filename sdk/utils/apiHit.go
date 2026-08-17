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
func RetryApiCall(
	method, apiURL string,
	headers map[string]string,
	username, password string,
	data interface{},
	reqType int,
	retryMax int,
	retryWaitMin, retryWaitMax time.Duration,
) (map[string]interface{}, error) {
	client := SharedHTTPClient(retryMax, retryWaitMin, retryWaitMax)

	var bodyReader io.Reader
	if method != "GET" && method != "get" {
		switch reqType {
		case variables.ContentTypeFormEncoded:
			formData, ok := data.(map[string]string)
			if !ok {
				return nil, fmt.Errorf("data must be of type map[string]string for form encoding")
			}
			formValues := url.Values{}
			for key, value := range formData {
				formValues.Set(key, value)
			}
			bodyReader = bytes.NewBufferString(formValues.Encode())
		case variables.ContentTypeText:
			rawData, ok := data.(string)
			if !ok {
				return nil, fmt.Errorf("data must be a string for Content-Type text/plain")
			}
			bodyReader = bytes.NewBufferString(rawData)
		default:
			if data == nil {
				bodyReader = bytes.NewBuffer(nil)
			} else {
				jsonData, err := json.Marshal(data)
				if err != nil {
					return nil, fmt.Errorf("error marshalling data: %v", err)
				}
				bodyReader = bytes.NewBuffer(jsonData)
			}
		}
	}

	var req *retryablehttp.Request
	var err error
	switch method {
	case "POST", "post":
		req, err = retryablehttp.NewRequest(http.MethodPost, apiURL, bodyReader)
	case "PUT", "put":
		req, err = retryablehttp.NewRequest(http.MethodPut, apiURL, bodyReader)
	case "GET", "get":
		req, err = retryablehttp.NewRequest(http.MethodGet, apiURL, nil)
	default:
		return nil, fmt.Errorf("invalid HTTP method: %s", method)
	}
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	// Add authentication if username and password are provided
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Set the Content-Type header
	if reqType == variables.ContentTypeFormEncoded {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	// Make the HTTP request with retry
	resp, err := client.Do(req)
	// utils.Debug(fmt.Sprintf("Response: %v", resp))
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}
	Info(fmt.Sprintf("API_RESPONSE status=%d keys=%d", resp.StatusCode, len(result)))
	result["ApistatusCode"] = int(resp.StatusCode)
	return result, nil
}

// ApiHit makes an API call using the shared HTTP client (no per-call goroutine/client).
func ApiHit(method, apiURL string, headers map[string]string, username, password string, data interface{}, reqType int) (map[string]interface{}, error) {
	return RetryApiCall(method, apiURL, headers, username, password, data, reqType, 0, 0, 0)
}
