package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/go-retryablehttp"
)

// Helper function (not in use)
func PrintFormattedRetryableRequest(req *retryablehttp.Request) {
	httpReq := req.Request

	// Print method and URL
	Debug(fmt.Sprintf("Method: %s", httpReq.Method))
	Debug(fmt.Sprintf("URL: %s", httpReq.URL.String()))

	// Print headers
	Debug("Headers:")
	for key, value := range httpReq.Header {
		Debug(fmt.Sprintf("  %s: %s", key, value))
	}

	// Capture and restore body
	var bodyBytes []byte
	var err error

	// if req.Body is nil but req.GetBody is defined, call it
	if httpReq.Body == nil && httpReq.GetBody != nil {
		httpReq.Body, err = httpReq.GetBody()
		if err != nil {
			Debug("Error restoring body from GetBody: " + err.Error())
			return
		}
	}

	if httpReq.Body != nil {
		bodyBytes, err = io.ReadAll(httpReq.Body)
		if err == nil {
			// Try pretty print
			var prettyBody bytes.Buffer
			err = json.Indent(&prettyBody, bodyBytes, "", "  ")
			if err == nil {
				Debug("Body:")
				Debug(prettyBody.String())
			} else {
				Debug(fmt.Sprintf("Body (raw): %s", string(bodyBytes)))
			}
			// Restore body
			httpReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		} else {
			Debug("Error reading request body: " + err.Error())
		}
	} else {
		Debug("No Body present in request.")
	}
}
