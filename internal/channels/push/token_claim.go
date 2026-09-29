package push

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

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
	AttemptCount(field string) (attemptCount int, exists bool, err error)
	ClaimedAt(field string) (claimedAt time.Time, exists bool, err error)
	Claim(field string) (claimID string, err error)
	ReclaimExpired(field string, cutoff time.Time) (bool, error)
	ClaimID(field string) (claimID string, exists bool, err error)
	Outcome(field string) (outcome string, exists bool, err error)
	RefreshClaim(field, claimID string) error
	SetAttemptCount(field, claimID string, attemptCount int) error
	SetTransactionID(field, claimID, transactionID string) error
	SetErrorMessage(field, claimID, outcome, errorMessage string) error
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

func (c *redisTokenClaims) AttemptCount(field string) (int, bool, error) {
	return redis.GetPushAttemptCount(c.hash, field, c.rdb)
}

func (c *redisTokenClaims) ClaimedAt(field string) (time.Time, bool, error) {
	return redis.GetPushClaimedAt(c.hash, field, c.rdb)
}

func (c *redisTokenClaims) Claim(field string) (string, error) {
	return redis.SetPushClaimKey(c.rdb, c.hash, field)
}

func (c *redisTokenClaims) ReclaimExpired(field string, cutoff time.Time) (bool, error) {
	return redis.ReclaimExpiredPushClaim(c.rdb, c.hash, field, cutoff)
}

func (c *redisTokenClaims) ClaimID(field string) (string, bool, error) {
	return redis.GetPushClaimID(c.hash, field, c.rdb)
}

func (c *redisTokenClaims) Outcome(field string) (string, bool, error) {
	return redis.GetPushOutcome(c.hash, field, c.rdb)
}

func (c *redisTokenClaims) RefreshClaim(field, claimID string) error {
	return redis.RefreshPushClaim(c.rdb, c.hash, field, claimID)
}

func (c *redisTokenClaims) SetAttemptCount(field, claimID string, attemptCount int) error {
	return redis.UpdatePushAttemptCount(c.rdb, c.hash, field, claimID, attemptCount)
}

func (c *redisTokenClaims) SetTransactionID(field, claimID, transactionID string) error {
	return redis.UpdatePushTransactionID(c.rdb, c.hash, field, claimID, transactionID)
}

func (c *redisTokenClaims) SetErrorMessage(field, claimID, outcome, errorMessage string) error {
	return redis.UpdatePushErrorMessage(c.rdb, c.hash, field, claimID, outcome, errorMessage)
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
