package fcm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	fcmSendTimeout    = 30 * time.Second
	fcmResponseLimit  = 1 << 20
	fcmSendURLPattern = "https://fcm.googleapis.com/v1/projects/%s/messages:send"
)

type accessTokenSource interface {
	Token(ctx context.Context, client string, cfg ClientConfig) (string, error)
}

// SendResponse contains only the provider fields needed for classification and
// auditing. Provider response messages are deliberately omitted because they
// can echo sensitive request data.
type SendResponse struct {
	HTTPStatus  int
	MessageID   string
	ErrorStatus string
	ErrorCode   string
}

type Sender struct {
	configs     map[string]ClientConfig
	tokenSource accessTokenSource
	httpClient  *http.Client
}

func NewSender(configs map[string]ClientConfig, tokenSource accessTokenSource, httpClient *http.Client) (*Sender, error) {
	if len(configs) == 0 {
		return nil, errors.New("FCM client configuration is required")
	}

	if tokenSource == nil {
		return nil, errors.New("FCM token source is required")
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: fcmSendTimeout}
	}

	return &Sender{configs: configs, tokenSource: tokenSource, httpClient: httpClient}, nil
}

// Send performs exactly one FCM provider attempt. Retry policy is owned by the
// caller so attempt counts and ledger transitions remain explicit.
func (s *Sender) Send(ctx context.Context, client string, payload SendRequest) (SendResponse, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, fcmSendTimeout)
	defer cancel()

	cfg, err := ResolveClientConfig(s.configs, client)
	if err != nil {
		return SendResponse{}, PermanentAttemptError("FCM_CONFIG_NOT_FOUND", err)
	}

	if strings.TrimSpace(payload.Message.Token) == "" || len(payload.Message.Data) == 0 {
		return SendResponse{}, PermanentAttemptError("FCM_PAYLOAD_INCOMPLETE", errors.New("FCM data-only payload is incomplete"))
	}

	accessToken, err := s.tokenSource.Token(attemptCtx, client, cfg)
	if err != nil {
		return SendResponse{}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return SendResponse{}, PermanentAttemptError("FCM_PAYLOAD_INVALID", fmt.Errorf("marshal FCM request for client %q: %w", strings.ToLower(strings.TrimSpace(client)), err))
	}

	endpoint := fmt.Sprintf(fcmSendURLPattern, url.PathEscape(cfg.ProjectID))
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SendResponse{}, PermanentAttemptError("FCM_REQUEST_INVALID", fmt.Errorf("create FCM send request for client %q: %w", strings.ToLower(strings.TrimSpace(client)), err))
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return SendResponse{}, RetryableAttemptError("FCM_TRANSPORT_ERROR", fmt.Errorf("FCM send request failed for client %q: %w", strings.ToLower(strings.TrimSpace(client)), err))
	}
	defer resp.Body.Close()

	limitedBody, err := io.ReadAll(io.LimitReader(resp.Body, fcmResponseLimit))
	if err != nil {
		readErr := fmt.Errorf("read FCM response for client %q: %w", strings.ToLower(strings.TrimSpace(client)), err)
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return SendResponse{HTTPStatus: resp.StatusCode}, PermanentAttemptError("FCM_ACCEPTANCE_UNKNOWN", readErr)
		}
		return SendResponse{HTTPStatus: resp.StatusCode}, RetryableAttemptError("FCM_RESPONSE_UNREADABLE", readErr)
	}

	result := SendResponse{HTTPStatus: resp.StatusCode}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		var success struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(limitedBody, &success); err != nil {
			return result, PermanentAttemptError("FCM_ACCEPTANCE_UNKNOWN", fmt.Errorf("decode FCM success response for client %q: %w", strings.ToLower(strings.TrimSpace(client)), err))
		}

		result.MessageID = strings.TrimSpace(success.Name)
		if result.MessageID == "" {
			return result, PermanentAttemptError("FCM_ACCEPTANCE_UNKNOWN", fmt.Errorf("FCM success response omitted message name for client %q", strings.ToLower(strings.TrimSpace(client))))
		}

		return result, nil
	}

	var failure struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(limitedBody, &failure); err == nil {
		result.ErrorStatus = strings.TrimSpace(failure.Error.Status)

		for _, detail := range failure.Error.Details {
			if code := strings.TrimSpace(detail.ErrorCode); code != "" {
				result.ErrorCode = code
				break
			}
		}
	}

	return result, nil
}
