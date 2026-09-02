package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

const commIdentitySeparator = "|"

func SignCommIdentity(secret, username, role string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.TrimSpace(username) + commIdentitySeparator + strings.TrimSpace(role)))
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyCommIdentity(secret, username, role, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}

	expected := SignCommIdentity(secret, username, role)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}
