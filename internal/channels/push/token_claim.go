package push

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	goredis "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

// tokenClaimStore is the Redis (or test double) claim authority for one PUSH
// delivery identity field: {GenerateRedisKeyForRequest}:{tokenFingerprint}.
type tokenClaimStore interface {
	Get(field string) (exists bool, transactionID, errorMessage string, err error)
	Claim(field string) error
	ReclaimBlank(field string) (bool, error)
	SetTransactionID(field, transactionID string) error
	SetErrorMessage(field, errorMessage string) error
}

type redisTokenClaims struct {
	rdb  *goredis.Client
	hash string
}

func newRedisTokenClaims() (tokenClaimStore, error) {
	if redis.RDB == nil {
		return nil, fmt.Errorf("PUSH redis client is nil")
	}
	hash := strings.TrimSpace(config.Configs.CommIdempotentKey)
	if hash == "" {
		return nil, fmt.Errorf("PUSH CommIdempotentKey is empty")
	}
	return &redisTokenClaims{rdb: redis.RDB, hash: hash}, nil
}

func (c *redisTokenClaims) Get(field string) (bool, string, string, error) {
	return redis.GetMobileDataFromRedis(c.hash, field, c.rdb)
}

func (c *redisTokenClaims) Claim(field string) error {
	return redis.SetMobileChannelKey(c.rdb, c.hash, field)
}

func (c *redisTokenClaims) ReclaimBlank(field string) (bool, error) {
	return redis.ReclaimBlankMobileChannelKey(c.rdb, c.hash, field)
}

func (c *redisTokenClaims) SetTransactionID(field, transactionID string) error {
	return redis.UpdateTransactionId(c.rdb, c.hash, field, transactionID)
}

func (c *redisTokenClaims) SetErrorMessage(field, errorMessage string) error {
	return redis.UpdateErrorMessage(c.rdb, c.hash, field, errorMessage)
}

// FingerprintToken returns a non-reversible SHA-256 hex fingerprint of a device
// token. Raw tokens must never be stored in Redis fields, audit maps, or logs.
func FingerprintToken(deviceToken string) (string, error) {
	deviceToken = strings.TrimSpace(deviceToken)
	if deviceToken == "" {
		return "", errors.New("PUSH device token is required")
	}
	digest := sha256.Sum256([]byte(deviceToken))
	return hex.EncodeToString(digest[:]), nil
}

// TokenRedisField builds the per-token Redis hash field.
// Base identity matches SMS GenerateRedisKeyForRequest (EventId when set).
func TokenRedisField(request sdkModels.CommApiRequestBody, tokenFingerprint string) string {
	base := channelHelper.GenerateRedisKeyForRequest(request)
	return base + ":" + strings.TrimSpace(tokenFingerprint)
}
