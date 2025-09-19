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

// RetryApiCall handles retries for an API call
func RetryApiCall(
	method, apiURL string,
	headers map[string]string,
	username, password string,
	data interface{},
	reqType int,
	retryMax int,
	retryWaitMin, retryWaitMax time.Duration,
) (map[string]interface{}, error) {
	client := retryablehttp.NewClient()
	client.RetryMax = retryMax
	client.RetryWaitMin = retryWaitMin
	client.RetryWaitMax = retryWaitMax

	var bodyBuffer *bytes.Buffer
	// Prepare the request body
	if reqType == variables.ContentTypeFormEncoded {
		formData, ok := data.(map[string]string)
		if !ok {
			return nil, fmt.Errorf("data must be of type map[string]string for form encoding")
		}
		formValues := url.Values{}
		for key, value := range formData {
			formValues.Set(key, value)
		}
		bodyBuffer = bytes.NewBufferString(formValues.Encode())
	} else if reqType == variables.ContentTypeText {
		rawData, ok := data.(string)
		if !ok {
			return nil, fmt.Errorf("data must be a string for Content-Type text/plain")
		}
		bodyBuffer = bytes.NewBufferString(rawData)
	} else {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("error marshalling data: %v", err)
		}
		bodyBuffer = bytes.NewBuffer(jsonData)
	}

	// Create the request based on the HTTP method
	var req *retryablehttp.Request
	var err error
	switch method {
	case "POST", "post":
		req, err = retryablehttp.NewRequest(http.MethodPost, apiURL, bodyBuffer)
	case "PUT", "put":
		req, err = retryablehttp.NewRequest(http.MethodPut, apiURL, bodyBuffer)
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
	fmt.Println("Response: ", resp)
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
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}
	Info(fmt.Sprintf("API_RESPONSE: %v", result))
	result["ApistatusCode"] = int(resp.StatusCode)
	return result, nil
}

// ApiHit makes an API call with optional retries
func ApiHit(method, apiURL string, headers map[string]string, username, password string, data interface{}, reqType int) (map[string]interface{}, error) {
	// Handle retry logic in a goroutine
	resultChan := make(chan map[string]interface{})
	errChan := make(chan error)
	go func() {
		result, err := RetryApiCall(method, apiURL, headers, username, password, data, reqType, 0, 0*time.Second, 0*time.Second) //TODO: Retry api call is paused for now
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- result
	}()

	// Wait for the result or error
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		return nil, err
	}
}
