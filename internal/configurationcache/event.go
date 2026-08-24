package configurationcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	redisclient "github.com/redis/go-redis/v9"
)

const invalidationChannelPrefix = "comm:configuration-cache:invalidate:"

type templateInvalidationEvent struct {
	TemplateVersion int64 `json:"templateVersion"`
}


// invalidationChannel returns the invalidation channel for the given environment
func invalidationChannel(environment string) (string, error) {
	environment = strings.TrimSpace(environment)
	if environment == "" {
		return "", errors.New("environment is required for cache invalidation")
	}

	return invalidationChannelPrefix + environment, nil
}


// PublishTemplateInvalidation publishes a template invalidation event
func PublishTemplateInvalidation(ctx context.Context, client *redisclient.Client, environment string, templateVersion int64) error {
	if client == nil {
		return errors.New("redis client is required")
	}

	if templateVersion <= 0 {
		return errors.New("template version must be positive")
	}
	
	channel, err := invalidationChannel(environment)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(templateInvalidationEvent{TemplateVersion: templateVersion})
	if err != nil {
		return fmt.Errorf("marshal cache invalidation: %w", err)
	}

	if err := client.Publish(ctx, channel, payload).Err(); err != nil {
		return fmt.Errorf("publish cache invalidation: %w", err)
	}

	return nil
}
