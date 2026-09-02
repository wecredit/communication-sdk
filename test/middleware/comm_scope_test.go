package middleware_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/middleware"
)

func TestResolveCommAdminScope(t *testing.T) {
	cfg := middleware.NewCommScopeConfig("marketing", "marketing_")

	tests := []struct {
		name         string
		role         string
		username     string
		unrestricted bool
		allowed      string
		wantErr      bool
	}{
		{
			name:         "marketing is unrestricted",
			role:         "marketing",
			username:     "marketing@wecredit.co.in",
			unrestricted: true,
		},
		{
			name:     "marketing_wecredit is scoped",
			role:     "marketing_wecredit",
			username: "wecredit@wecredit.co.in",
			allowed:  "wecredit",
		},
		{
			name:     "marketing_zapcash is scoped",
			role:     "marketing_zapcash",
			username: "zapcash@wecredit.co.in",
			allowed:  "zapcash",
		},
		{
			name:    "marketing_wecredit does not inherit marketing access",
			role:    "marketing_wecredit",
			wantErr: false,
		},
		{
			name:    "unknown role is rejected",
			role:    "sales",
			wantErr: true,
		},
		{
			name:         "username fallback for marketing",
			username:     "marketing@wecredit.co.in",
			unrestricted: true,
		},
		{
			name:     "username fallback for marketing_wecredit",
			username: "marketing_wecredit@wecredit.co.in",
			allowed:  "wecredit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, err := middleware.ResolveCommAdminScope(tt.role, tt.username, cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if scope.Unrestricted != tt.unrestricted {
				t.Fatalf("unrestricted = %v, want %v", scope.Unrestricted, tt.unrestricted)
			}
			if tt.allowed != "" {
				if len(scope.AllowedClients) != 1 || scope.AllowedClients[0] != tt.allowed {
					t.Fatalf("allowed clients = %#v, want [%q]", scope.AllowedClients, tt.allowed)
				}
			}
		})
	}
}

func TestApplyClientListFilter(t *testing.T) {
	cfg := middleware.NewCommScopeConfig("marketing", "marketing_")
	scope, err := middleware.ResolveCommAdminScope("marketing_wecredit", "wecredit@wecredit.co.in", cfg)
	if err != nil {
		t.Fatalf("resolve scope: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	middleware.SetCommAdminScope(context, scope)

	client, err := middleware.ApplyClientListFilter(context, "")
	if err != nil {
		t.Fatalf("apply filter: %v", err)
	}
	if client != "wecredit" {
		t.Fatalf("client = %q, want wecredit", client)
	}

	if _, err := middleware.ApplyClientListFilter(context, "zapcash"); err == nil {
		t.Fatal("expected forbidden for mismatched client query")
	}
}

func TestEnforceClientAccess(t *testing.T) {
	cfg := middleware.NewCommScopeConfig("marketing", "marketing_")
	scope, err := middleware.ResolveCommAdminScope("marketing_zapcash", "zapcash@wecredit.co.in", cfg)
	if err != nil {
		t.Fatalf("resolve scope: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	middleware.SetCommAdminScope(context, scope)

	if err := middleware.EnforceClientAccess(context, "zapcash"); err != nil {
		t.Fatalf("expected zapcash access: %v", err)
	}
	if err := middleware.EnforceClientAccess(context, "wecredit"); err == nil {
		t.Fatal("expected forbidden for wecredit")
	}
}
