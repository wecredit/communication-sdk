package utils_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func TestHTTPClientWithTimeoutLeavesSharedClientAt30s(t *testing.T) {
	shared := utils.SharedHTTPClient(0, 0, 0)
	slow := utils.HTTPClientWithTimeout(0, 0, 0, 60*time.Second)

	if shared.HTTPClient == nil || slow.HTTPClient == nil {
		t.Fatal("missing http client")
	}
	if shared.HTTPClient.Timeout != 30*time.Second {
		t.Fatalf("shared timeout = %s, want 30s", shared.HTTPClient.Timeout)
	}
	if slow.HTTPClient.Timeout != 60*time.Second {
		t.Fatalf("override timeout = %s, want 60s", slow.HTTPClient.Timeout)
	}
	if shared.HTTPClient == slow.HTTPClient {
		t.Fatal("timeout override shares the http.Client value")
	}
	if shared.HTTPClient.Transport != slow.HTTPClient.Transport {
		t.Fatal("timeout override should reuse the shared transport")
	}
	if shared.HTTPClient.Timeout != 30*time.Second {
		t.Fatal("building the override changed the shared timeout")
	}
}

func TestApiHitWithTimeoutUsesCallerDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	_, err := utils.ApiHitWithTimeout(http.MethodGet, srv.URL, nil, "", "", nil, variables.ContentTypeJSON, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "Client.Timeout") {
		t.Fatalf("short timeout err = %v", err)
	}

	got, err := utils.ApiHitWithTimeout(http.MethodGet, srv.URL, nil, "", "", nil, variables.ContentTypeJSON, time.Second)
	if err != nil {
		t.Fatalf("long timeout err = %v", err)
	}
	if utils.HTTPStatusFromResponse(got) != http.StatusOK {
		t.Fatalf("status = %d", utils.HTTPStatusFromResponse(got))
	}
}
