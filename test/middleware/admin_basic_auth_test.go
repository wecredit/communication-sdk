package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/middleware"
)

func TestNewAdminBasicAuthRejectsMissingConfiguration(t *testing.T) {
	if _, err := middleware.NewAdminBasicAuth("", "password"); err == nil {
		t.Fatal("missing username was accepted")
	}
	if _, err := middleware.NewAdminBasicAuth("username", ""); err == nil {
		t.Fatal("missing password was accepted")
	}
}

func TestAdminBasicAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth, err := middleware.NewAdminBasicAuth("gateway", "strong-secret")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}

	tests := []struct {
		name       string
		username   string
		password   string
		setAuth    bool
		wantStatus int
	}{
		{name: "valid", username: "gateway", password: "strong-secret", setAuth: true, wantStatus: http.StatusNoContent},
		{name: "wrong password", username: "gateway", password: "wrong", setAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "missing header", wantStatus: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/admin", auth, func(c *gin.Context) { c.Status(http.StatusNoContent) })

			request := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if test.setAuth {
				request.SetBasicAuth(test.username, test.password)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantStatus == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("unauthorized response is missing WWW-Authenticate")
			}
		})
	}
}
