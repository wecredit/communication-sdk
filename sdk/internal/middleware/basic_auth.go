package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

// Define a key type to avoid context key collisions
type contextKey string

const usernameContextKey contextKey = "username"

// BasicAuthMiddleware validates Base64-encoded username and password
func BasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Basic ") {
			response := sdkModels.CommApiErrorResponseBody{
				StatusCode:    http.StatusUnauthorized,
				StatusMessage: "Unauthorized",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Decode the Base64 username:password
		encodedCredentials := strings.TrimPrefix(authHeader, "Basic ")
		decodedBytes, err := base64.StdEncoding.DecodeString(encodedCredentials)
		if err != nil {
			response := sdkModels.CommApiErrorResponseBody{
				StatusCode:    http.StatusUnauthorized,
				StatusMessage: "Unauthorized",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(response)
			return
		}
		decodedCredentials := string(decodedBytes)

		// Split the username and password
		parts := strings.SplitN(decodedCredentials, ":", 2)
		if len(parts) != 2 {
			response := sdkModels.CommApiErrorResponseBody{
				StatusCode:    http.StatusUnauthorized,
				StatusMessage: "Unauthorized",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(response)
			return
		}
		username, password := parts[0], parts[1]

		// Collecting BasicAuthData
		authDetails, _ := cache.GetCache().Get(cache.AuthDetails)

		// Validate the credentials
		isValid := false
		for _, data := range authDetails {
			// extract username and password from the map
			usernameFromData, _ := data["username"].(string)
			passwordFromData, _ := data["password"].(string)

			// Validate headers username and password
			if usernameFromData == username && passwordFromData == password {
				isValid = true
				break
			}

		}

		// If username or password doesn't match, return Unauthorized
		if !isValid {
			response := sdkModels.CommApiErrorResponseBody{
				StatusCode:    http.StatusUnauthorized,
				StatusMessage: "Unauthorized",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Store the username in the context and proceed to the next handler
		ctx := context.WithValue(r.Context(), usernameContextKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
