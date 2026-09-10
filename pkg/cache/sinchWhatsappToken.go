package cache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wecredit/communication-sdk/config"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
	"golang.org/x/sync/singleflight"
)

// Sinch WhatsApp token cache is in-process / per ECS task (not cross-task shared).
// Fetch, proactive refresh, and 401-driven invalidate+refetch all go through
// singleflight so concurrent senders do not stampede the token endpoint.

type waTokenEntry struct {
	accessToken  string
	refreshToken string
	expiresAt    time.Time
}

var (
	waTokenMu    sync.RWMutex
	waTokenStore = map[string]*waTokenEntry{}
	waTokenSF    singleflight.Group
)

// ResetSinchWhatsappTokenCacheForTest clears in-process WA token state (tests only).
func ResetSinchWhatsappTokenCacheForTest() {
	waTokenMu.Lock()
	waTokenStore = map[string]*waTokenEntry{}
	waTokenMu.Unlock()
}

// InvalidateSinchWhatsappToken drops the cached token for client so the next
// GetSinchWhatsappAccessToken refetches (via singleflight).
func InvalidateSinchWhatsappToken(client string) {
	key := waTokenCacheKey(client)
	waTokenMu.Lock()
	delete(waTokenStore, key)
	waTokenMu.Unlock()
}

// GetSinchWhatsappAccessToken returns a valid access token for client, fetching
// once under singleflight on miss or expiry.
func GetSinchWhatsappAccessToken(client string) (string, error) {
	key := waTokenCacheKey(client)
	if token, ok := peekWAToken(key); ok {
		return token, nil
	}

	v, err, _ := waTokenSF.Do(key, func() (interface{}, error) {
		if token, ok := peekWAToken(key); ok {
			return token, nil
		}

		tok, err := fetchSinchWhatsappToken(client)
		if err != nil {
			return nil, err
		}

		storeWAToken(key, tok)
		scheduleWATokenRefresh(client, key, tok)
		return tok.AccessToken, nil
	})

	if err != nil {
		return "", err
	}

	return v.(string), nil
}

func peekWAToken(key string) (string, bool) {
	waTokenMu.RLock()
	defer waTokenMu.RUnlock()
	entry, ok := waTokenStore[key]
	if !ok || entry == nil || entry.accessToken == "" {
		return "", false
	}

	// Refresh skew: treat as expired 5s before wall expiry (same idea as RCS sinchToken).
	if time.Now().After(entry.expiresAt.Add(-5 * time.Second)) {
		return "", false
	}

	return entry.accessToken, true
}

func storeWAToken(key string, tok *extapimodels.SinchTokenResponse) {
	expiresIn := tok.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 60
	}

	waTokenMu.Lock()
	waTokenStore[key] = &waTokenEntry{
		accessToken:  tok.AccessToken,
		refreshToken: tok.RefreshToken,
		expiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}

	waTokenMu.Unlock()
}

func scheduleWATokenRefresh(client, key string, tok *extapimodels.SinchTokenResponse) {
	expiresIn := tok.ExpiresIn
	if expiresIn <= 5 {
		return
	}

	wait := time.Duration(expiresIn-5) * time.Second
	go func() {
		time.Sleep(wait)
		_, _, _ = waTokenSF.Do(key+":refresh", func() (interface{}, error) {
			waTokenMu.RLock()
			entry := waTokenStore[key]
			waTokenMu.RUnlock()

			if entry == nil {
				return nil, nil
			}

			var newTok *extapimodels.SinchTokenResponse
			var err error
			if strings.TrimSpace(entry.refreshToken) != "" {
				newTok, err = refreshSinchWhatsappToken(entry.refreshToken)
			}

			if err != nil || newTok == nil || newTok.AccessToken == "" {
				newTok, err = fetchSinchWhatsappToken(client)
			}

			if err != nil || newTok == nil || newTok.AccessToken == "" {
				return nil, err
			}

			storeWAToken(key, newTok)
			scheduleWATokenRefresh(client, key, newTok)
			return newTok.AccessToken, nil
		})
	}()
}

func waTokenCacheKey(client string) string {
	client = strings.ToLower(strings.TrimSpace(client))
	if client == "" {
		client = "default"
	}

	return "wa:" + client
}

func fetchSinchWhatsappToken(client string) (*extapimodels.SinchTokenResponse, error) {
	generateTokenURL := config.Configs.SinchWhatsappTokenApiUrl
	if generateTokenURL == "" {
		return nil, fmt.Errorf("SINCH_GENERATE_TOKEN_API_URL is not set")
	}

	tokenPayload := sinchWhatsappTokenPayload(client)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	tokenResponse, err := utils.ApiHit("POST", generateTokenURL, headers, "", "", tokenPayload, variables.ContentTypeFormEncoded)
	if err != nil {
		return nil, fmt.Errorf("sinch WA token request failed: %w", err)
	}

	tok := &extapimodels.SinchTokenResponse{}
	accessToken, _ := tokenResponse["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("failed to generate access token")
	}

	tok.AccessToken = accessToken
	if refresh, ok := tokenResponse["refresh_token"].(string); ok {
		tok.RefreshToken = refresh
	}

	if exp, ok := tokenResponse["expires_in"].(float64); ok {
		tok.ExpiresIn = int(exp)
	} else if exp, ok := tokenResponse["expires_in"].(int); ok {
		tok.ExpiresIn = exp
	}

	if rexp, ok := tokenResponse["refresh_expires_in"].(float64); ok {
		tok.RefreshExpiresIn = int(rexp)
	}

	if tt, ok := tokenResponse["token_type"].(string); ok {
		tok.TokenType = tt
	}

	utils.Info(fmt.Sprintf("Sinch WA token fetched for client=%s expires_in=%d", strings.ToLower(strings.TrimSpace(client)), tok.ExpiresIn))
	return tok, nil
}

func refreshSinchWhatsappToken(refreshToken string) (*extapimodels.SinchTokenResponse, error) {
	generateTokenURL := config.Configs.SinchWhatsappTokenApiUrl
	if generateTokenURL == "" {
		return nil, fmt.Errorf("SINCH_GENERATE_TOKEN_API_URL is not set")
	}

	tokenPayload := map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     config.Configs.SinchWhatsappClientId,
		"refresh_token": refreshToken,
	}

	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	tokenResponse, err := utils.ApiHit("POST", generateTokenURL, headers, "", "", tokenPayload, variables.ContentTypeFormEncoded)
	if err != nil {
		return nil, err
	}

	accessToken, _ := tokenResponse["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("failed to refresh access token")
	}

	tok := &extapimodels.SinchTokenResponse{AccessToken: accessToken}
	if refresh, ok := tokenResponse["refresh_token"].(string); ok {
		tok.RefreshToken = refresh
	}

	if exp, ok := tokenResponse["expires_in"].(float64); ok {
		tok.ExpiresIn = int(exp)
	}

	return tok, nil
}

func sinchWhatsappTokenPayload(client string) map[string]string {
	if client == variables.CreditSea {
		return map[string]string{
			"grant_type": config.Configs.SinchWhatsappGrantType,
			"client_id":  config.Configs.SinchWhatsappClientId,
			"username":   config.Configs.CreditSeaSinchWhatsappUsername,
			"password":   config.Configs.CreditSeaSinchWhatsappPassword,
		}
	}

	return map[string]string{
		"grant_type": config.Configs.SinchWhatsappGrantType,
		"client_id":  config.Configs.SinchWhatsappClientId,
		"username":   config.Configs.SinchWhatsappUserName,
		"password":   config.Configs.SinchWhatsappPassword,
	}
}
