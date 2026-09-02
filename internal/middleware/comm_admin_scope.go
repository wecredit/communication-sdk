package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var ErrCommIdentitySecretNotConfigured = errors.New("COMM_IDENTITY_SECRET is not configured")

const commAdminScopeContextKey = "commAdminScope"

func NewCommAdminScopeMiddleware(cfg CommScopeConfig, identitySecret string) (gin.HandlerFunc, error) {
	if identitySecret == "" {
		return nil, ErrCommIdentitySecretNotConfigured
	}

	return func(c *gin.Context) {
		role := strings.TrimSpace(c.GetHeader("X-Comm-Role"))
		username := strings.TrimSpace(c.GetHeader("X-Comm-Username"))
		signature := strings.TrimSpace(c.GetHeader("X-Comm-Identity-Signature"))

		if !VerifyCommIdentity(identitySecret, username, role, signature) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "invalid communication admin identity",
				},
			})
			return
		}

		scope, err := ResolveCommAdminScope(role, username, cfg)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "access denied for this client",
				},
			})
			return
		}

		c.Set(commAdminScopeContextKey, scope)
		c.Next()
	}, nil
}

func GetCommAdminScope(c *gin.Context) (CommAdminScope, bool) {
	value, exists := c.Get(commAdminScopeContextKey)
	if !exists {
		return CommAdminScope{}, false
	}

	scope, ok := value.(CommAdminScope)
	return scope, ok
}

func SetCommAdminScope(c *gin.Context, scope CommAdminScope) {
	c.Set(commAdminScopeContextKey, scope)
}

func ApplyClientListFilter(c *gin.Context, requestedClient string) (string, error) {
	scope, ok := GetCommAdminScope(c)
	if !ok {
		return "", ErrScopeForbidden
	}

	requestedClient = strings.ToLower(strings.TrimSpace(requestedClient))
	if scope.Unrestricted {
		return requestedClient, nil
	}

	if len(scope.AllowedClients) == 0 {
		return "", ErrScopeForbidden
	}

	allowedClient := scope.AllowedClients[0]
	if requestedClient != "" && requestedClient != allowedClient {
		return "", ErrScopeForbidden
	}

	return allowedClient, nil
}

func EnforceClientAccess(c *gin.Context, client string) error {
	scope, ok := GetCommAdminScope(c)
	if !ok {
		return ErrScopeForbidden
	}

	if clientAllowed(scope, client) {
		return nil
	}

	return ErrScopeForbidden
}
