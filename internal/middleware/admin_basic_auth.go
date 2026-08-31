package middleware

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var ErrAdminBasicAuthNotConfigured = errors.New("communication admin Basic Auth credentials are not configured")

// NewAdminBasicAuth validates static, environment-backed service credentials.
// It is intentionally independent of the database-backed SDK client credentials.
func NewAdminBasicAuth(expectedUsername, expectedPassword string) (gin.HandlerFunc, error) {
	if expectedUsername == "" || expectedPassword == "" {
		return nil, ErrAdminBasicAuthNotConfigured
	}

	return func(c *gin.Context) {
		username, password, ok := c.Request.BasicAuth()
		if !ok || !constantTimeEqual(username, expectedUsername) || !constantTimeEqual(password, expectedPassword) {
			c.Header("WWW-Authenticate", `Basic realm="communication-admin", charset="UTF-8"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "valid communication admin credentials are required",
				},
			})
			return
		}

		c.Next()
	}, nil
}

func constantTimeEqual(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
