package utils

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

var (
	sharedHTTPOnce sync.Once
	sharedHTTP     *retryablehttp.Client
)

// SharedHTTPClient returns a process-wide retryable HTTP client with a pooled transport.
// Callers must not mutate the returned client.
func SharedHTTPClient(retryMax int, retryWaitMin, retryWaitMax time.Duration) *retryablehttp.Client {
	sharedHTTPOnce.Do(func() {
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
		client := retryablehttp.NewClient()
		client.HTTPClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
		client.Logger = nil
		sharedHTTP = client
	})

	// Clone retry settings per call without recreating the transport.
	client := *sharedHTTP
	client.RetryMax = retryMax
	client.RetryWaitMin = retryWaitMin
	client.RetryWaitMax = retryWaitMax
	client.HTTPClient = sharedHTTP.HTTPClient
	return &client
}
