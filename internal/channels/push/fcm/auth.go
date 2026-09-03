package fcm

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const firebaseMessagingScope = "https://www.googleapis.com/auth/firebase.messaging"

type serviceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

// TokenProvider creates OAuth tokens from secret-manager-mounted service
// accounts and caches them independently for each client and Firebase project.
type TokenProvider struct {
	mu         sync.Mutex
	httpClient *http.Client
	tokens     map[string]cachedToken
	now        func() time.Time
}

func NewTokenProvider(httpClient *http.Client) *TokenProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &TokenProvider{
		httpClient: httpClient,
		tokens:     make(map[string]cachedToken),
		now:        time.Now,
	}
}

// Token returns a bearer token for client. The mutex intentionally covers a
// refresh so concurrent sends cannot stampede Google's token endpoint.
func (p *TokenProvider) Token(ctx context.Context, client string, cfg ClientConfig) (string, error) {
	client = strings.ToLower(strings.TrimSpace(client))
	if client == "" {
		return "", PermanentAttemptError("AUTH_CLIENT_MISSING", errors.New("FCM client is required for authentication"))
	}

	if strings.TrimSpace(cfg.ProjectID) == "" || strings.TrimSpace(cfg.CredentialsFile) == "" {
		return "", PermanentAttemptError("AUTH_CONFIG_INCOMPLETE", fmt.Errorf("FCM configuration is incomplete for client %q", client))
	}

	cacheKey := client + "\x00" + cfg.ProjectID + "\x00" + cfg.CredentialsFile
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if token, exists := p.tokens[cacheKey]; exists && now.Add(time.Minute).Before(token.expiresAt) {
		return token.value, nil
	}

	account, privateKey, err := loadServiceAccount(cfg, client)
	if err != nil {
		return "", PermanentAttemptError("AUTH_CREDENTIALS_INVALID", err)
	}

	assertion, err := signJWT(account, privateKey, now)
	if err != nil {
		return "", PermanentAttemptError("AUTH_ASSERTION_INVALID", fmt.Errorf("create FCM assertion for client %q: %w", client, err))
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", PermanentAttemptError("AUTH_REQUEST_INVALID", fmt.Errorf("create FCM token request for client %q: %w", client, err))
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", RetryableAttemptError("AUTH_TRANSPORT_ERROR", fmt.Errorf("request FCM access token for client %q: %w", client, err))
	}

	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		statusErr := fmt.Errorf("FCM token endpoint returned status %d for client %q", resp.StatusCode, client)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			return "", RetryableAttemptError(fmt.Sprintf("AUTH_HTTP_%d", resp.StatusCode), statusErr)
		}

		return "", PermanentAttemptError(fmt.Sprintf("AUTH_HTTP_%d", resp.StatusCode), statusErr)
	}

	var decoded tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&decoded); err != nil {
		return "", RetryableAttemptError("AUTH_RESPONSE_INVALID", fmt.Errorf("decode FCM token response for client %q: %w", client, err))
	}

	if strings.TrimSpace(decoded.AccessToken) == "" || decoded.ExpiresIn <= 0 {
		return "", RetryableAttemptError("AUTH_RESPONSE_INCOMPLETE", fmt.Errorf("FCM token endpoint returned an incomplete response for client %q", client))
	}

	p.tokens[cacheKey] = cachedToken{
		value:     decoded.AccessToken,
		expiresAt: now.Add(time.Duration(decoded.ExpiresIn) * time.Second),
	}

	return decoded.AccessToken, nil
}

func loadServiceAccount(cfg ClientConfig, client string) (serviceAccount, *rsa.PrivateKey, error) {
	credentialBytes, err := os.ReadFile(cfg.CredentialsFile)
	if err != nil {
		return serviceAccount{}, nil, fmt.Errorf("read FCM credentials for client %q: %w", client, err)
	}

	var account serviceAccount
	if err := json.Unmarshal(credentialBytes, &account); err != nil {
		return serviceAccount{}, nil, fmt.Errorf("parse FCM credentials for client %q: %w", client, err)
	}

	if strings.TrimSpace(account.ProjectID) != strings.TrimSpace(cfg.ProjectID) {
		return serviceAccount{}, nil, fmt.Errorf("FCM credential project does not match configured project for client %q", client)
	}

	if strings.TrimSpace(account.ClientEmail) == "" || strings.TrimSpace(account.PrivateKey) == "" || strings.TrimSpace(account.TokenURI) == "" {
		return serviceAccount{}, nil, fmt.Errorf("FCM credentials are incomplete for client %q", client)
	}

	tokenURL, err := url.Parse(account.TokenURI)
	if err != nil || tokenURL.Scheme != "https" || tokenURL.Host == "" {
		return serviceAccount{}, nil, fmt.Errorf("FCM token URI is invalid for client %q", client)
	}

	privateKey, err := parseRSAPrivateKey(account.PrivateKey)
	if err != nil {
		return serviceAccount{}, nil, fmt.Errorf("parse FCM private key for client %q: %w", client, err)
	}

	return account, privateKey, nil
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("private key is not PEM encoded")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("private key is not valid PKCS#8 or PKCS#1")
	}

	return key, nil
}

func signJWT(account serviceAccount, privateKey *rsa.PrivateKey, now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}

	claims, err := json.Marshal(map[string]interface{}{
		"iss":   account.ClientEmail,
		"scope": firebaseMessagingScope,
		"aud":   account.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})

	if err != nil {
		return "", err
	}

	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])

	if err != nil {
		return "", err
	}

	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
