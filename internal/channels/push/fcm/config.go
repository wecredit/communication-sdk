package fcm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ClientConfig identifies one client's isolated Firebase project and the
// secret-manager-mounted service-account file used to authenticate to it.
type ClientConfig struct {
	ProjectID       string `json:"projectId"`
	CredentialsFile string `json:"credentialsFile"`
}

// ParseClientConfigs parses FCM_CLIENT_CONFIG_JSON. Client keys are normalized
// to lowercase so request casing cannot select a different Firebase project.
func ParseClientConfigs(raw string) (map[string]ClientConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("FCM client configuration is empty")
	}

	var decoded map[string]ClientConfig
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("parse FCM client configuration: %w", err)
	}

	if len(decoded) == 0 {
		return nil, errors.New("FCM client configuration has no clients")
	}

	configs := make(map[string]ClientConfig, len(decoded))
	for client, cfg := range decoded {
		normalizedClient := strings.ToLower(strings.TrimSpace(client))
		if normalizedClient == "" {
			return nil, errors.New("FCM client configuration contains an empty client name")
		}

		if _, exists := configs[normalizedClient]; exists {
			return nil, fmt.Errorf("FCM client configuration contains duplicate client %q", normalizedClient)
		}

		cfg.ProjectID = strings.TrimSpace(cfg.ProjectID)
		cfg.CredentialsFile = strings.TrimSpace(cfg.CredentialsFile)
		if cfg.ProjectID == "" {
			return nil, fmt.Errorf("FCM projectId is required for client %q", normalizedClient)
		}

		if cfg.CredentialsFile == "" {
			return nil, fmt.Errorf("FCM credentialsFile is required for client %q", normalizedClient)
		}

		configs[normalizedClient] = cfg
	}

	return configs, nil
}

// ResolveClientConfig returns the Firebase configuration assigned to client.
func ResolveClientConfig(configs map[string]ClientConfig, client string) (ClientConfig, error) {
	normalizedClient := strings.ToLower(strings.TrimSpace(client))
	if normalizedClient == "" {
		return ClientConfig{}, errors.New("FCM client is required")
	}

	cfg, exists := configs[normalizedClient]
	if !exists {
		return ClientConfig{}, fmt.Errorf("FCM configuration not found for client %q", normalizedClient)
	}

	return cfg, nil
}
