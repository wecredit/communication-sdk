package redis

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/internal/models/redisModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

// ClaimCollectionAfternoonKey reserves one ZapCash afternoon collection RCS
// dispatch. It is a standalone Redis string so it never shares lifecycle with
// the legacy COMM_IDEMPOTENT_KEY hash.
func ClaimCollectionAfternoonKey(rdb *redis.Client, key, claimToken string, ttl time.Duration) (bool, error) {
	claimed, err := rdb.SetNX(context.Background(), key, claimToken, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("claim collection afternoon key %s: %w", key, err)
	}
	return claimed, nil
}

// ReleaseCollectionAfternoonKey removes a claim only when it is still owned
// by claimToken. This avoids deleting a later claimant after an expiry race.
func ReleaseCollectionAfternoonKey(rdb *redis.Client, key, claimToken string) (bool, error) {
	const script = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`
	released, err := rdb.Eval(context.Background(), script, []string{key}, claimToken).Int()
	if err != nil {
		return false, fmt.Errorf("release collection afternoon key %s: %w", key, err)
	}
	return released == 1, nil
}

func GetPushClaimedAt(commIdempotentKey, redisKey string, rdb *redis.Client) (time.Time, bool, error) {
	value, err := rdb.HGet(context.Background(), commIdempotentKey, redisKey).Result()
	if err == redis.Nil {
		return time.Time{}, false, nil
	}

	if err != nil {
		return time.Time{}, false, err
	}

	var data redisModels.MobileChannelRedisData
	if err := json.Unmarshal([]byte(value), &data); err != nil || data.ClaimedAtUnix <= 0 {
		return time.Time{}, true, nil
	}

	return time.Unix(data.ClaimedAtUnix, 0), true, nil
}

func GetPushAttemptCount(commIdempotentKey, redisKey string, rdb *redis.Client) (int, bool, error) {
	value, err := rdb.HGet(context.Background(), commIdempotentKey, redisKey).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	var data redisModels.MobileChannelRedisData
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return 0, true, nil
	}
	return data.AttemptCount, true, nil
}

func GetPushClaimID(commIdempotentKey, redisKey string, rdb *redis.Client) (string, bool, error) {
	value, err := rdb.HGet(context.Background(), commIdempotentKey, redisKey).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var data redisModels.MobileChannelRedisData
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return "", true, nil
	}
	return data.ClaimID, true, nil
}

func GetPushOutcome(commIdempotentKey, redisKey string, rdb *redis.Client) (string, bool, error) {
	value, err := rdb.HGet(context.Background(), commIdempotentKey, redisKey).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var data redisModels.MobileChannelRedisData
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return "", true, nil
	}
	return data.Outcome, true, nil
}

func SetPushClaimKey(rdb *redis.Client, commIdempotentKey, redisKey string) (string, error) {
	claimIDBytes := make([]byte, 16)
	if _, err := rand.Read(claimIDBytes); err != nil {
		return "", fmt.Errorf("generate PUSH claim id: %w", err)
	}
	claimID := fmt.Sprintf("%x", claimIDBytes)
	data, err := json.Marshal(redisModels.MobileChannelRedisData{
		ClaimedAtUnix: time.Now().UTC().Unix(),
		ClaimID:       claimID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal PUSH claim: %w", err)
	}

	created, err := rdb.HSetNX(context.Background(), commIdempotentKey, redisKey, string(data)).Result()
	if err != nil {
		return "", err
	}

	if !created {
		return "", fmt.Errorf("key %s already exists in redis", redisKey)
	}

	return claimID, nil
}

func ReclaimExpiredPushClaim(rdb *redis.Client, commIdempotentKey, redisKey string, cutoff time.Time) (bool, error) {
	const script = `
local val = redis.call('HGET', KEYS[1], ARGV[1])
if val == false or string.sub(val, 1, 1) ~= '{' then return 0 end
local claimed = string.match(val, '"claimedAtUnix"%s*:%s*(%d+)')
if claimed == nil or tonumber(claimed) > tonumber(ARGV[2]) then return 0 end
if string.match(val, '"transactionId"%s*:%s*"[^"]+"') then return 0 end
if string.match(val, '"errorMessage"%s*:%s*"[^"]+"') then return 0 end
return redis.call('HDEL', KEYS[1], ARGV[1])
`

	result, err := rdb.Eval(context.Background(), script, []string{commIdempotentKey}, redisKey, fmt.Sprintf("%d", cutoff.UTC().Unix())).Int()
	if err != nil {
		return false, fmt.Errorf("reclaim expired PUSH claim %s: %w", redisKey, err)
	}

	return result > 0, nil
}

// Function to store data into redis from db
func StoreDataInRedis(query string, db *gorm.DB, RDB *redis.Client, redisKey string) error {
	pipe := RDB.Pipeline()
	// Execute the query and fetch the result
	var result []string
	utils.Info("Running dedupe query..")
	err := db.Raw(query).Scan(&result).Error
	if err != nil {
		return fmt.Errorf("failed to execute query: %v", err) // You might want to handle this differently
	}
	utils.Info("Dedupe query execution completed.")

	// // Store the JSON data in Redis
	ctx := context.Background()

	pipe.SAdd(ctx, redisKey, result)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store data in Redis: %v", err) // You might want to handle this differently
	}

	utils.Info(fmt.Sprintf("Data successfully stored in Redis under key '%s': ", redisKey))
	return nil
}

func InitCreditSeaCounter(ctx context.Context, redisClient *redis.Client, key string, initialValue int) error {
	ok, err := redisClient.SetNX(ctx, key, initialValue, 0).Result()
	if err != nil {
		return err
	}
	if !ok {
		// Key already exists, no need to init
		return nil
	}
	return nil
}

func IncrementCreditSeaCounter(ctx context.Context, redisClient *redis.Client, key string) error {
	return redisClient.Incr(ctx, key).Err()
}

func GetCreditSeaCounter(ctx context.Context, redisClient *redis.Client, key string) (int, error) {
	val, err := redisClient.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil // key not found
	}
	return val, err
}

func ResetCreditSeaCounter(ctx context.Context, redisClient *redis.Client, key string) error {
	return redisClient.Set(ctx, key, 0, 0).Err()
}

// Check if mobile_channel exists and return both transactionId and errorMessage if present
func GetMobileDataFromRedis(CommIdempotentKey string, redisKey string, rdb *redis.Client) (bool, string, string, error) {
	ctx := context.Background()
	val, err := rdb.HGet(ctx, CommIdempotentKey, redisKey).Result()
	if err == redis.Nil {
		utils.Info(fmt.Sprintf("[redis]: %s does not exist. Proceed for communication", redisKey))
		return false, "", "", nil
	}
	if err != nil {
		return false, "", "", err
	}

	// Try to parse as JSON first (new format)
	var data redisModels.MobileChannelRedisData
	if err := json.Unmarshal([]byte(val), &data); err == nil {
		utils.Debug(fmt.Sprintf("[redis]: %s exists. TransactionId: %s, ErrorMessage: %s", redisKey, data.TransactionId, data.ErrorMessage))
		return true, data.TransactionId, data.ErrorMessage, nil
	}

	// Fallback to old format (single string value)
	// If it's not JSON, treat it as the old format where everything was stored as transactionId
	utils.Debug(fmt.Sprintf("[redis]: %s exists. TransactionId: %s", redisKey, val))
	return true, val, "", nil
}

// 1. Create a field (mobile_channel) inside CommIdempotentKey with blank value
// Returns error if key already exists (HSetNX returns false)
func SetMobileChannelKey(RDB *redis.Client, commIdempotentKey, redisKey string) error {
	ctx := context.Background()
	created, err := RDB.HSetNX(ctx, commIdempotentKey, redisKey, "").Result()
	if err != nil {
		utils.Error(fmt.Errorf("failed to set key %s in redis: %v", redisKey, err))
		return err
	}
	if !created {
		utils.Info(fmt.Sprintf("Key %s already exists in hash %s", redisKey, commIdempotentKey))
		return fmt.Errorf("key %s already exists in redis", redisKey)
	}
	utils.Info(fmt.Sprintf("Key %s created in hash %s with blank value", redisKey, commIdempotentKey))
	return nil
}

// ReclaimBlankMobileChannelKey deletes a hash field only when it is still an
// orphan blank claim (empty string or JSON with no transactionId/errorMessage).
// Used for EventId-keyed marketing sends so a crash after HSetNX does not
// permanently block retries. Returns true when the field was deleted.
func ReclaimBlankMobileChannelKey(RDB *redis.Client, commIdempotentKey, redisKey string) (bool, error) {
	ctx := context.Background()
	const script = `
local val = redis.call('HGET', KEYS[1], ARGV[1])
if val == false then
  return 0
end
if val == '' then
  return redis.call('HDEL', KEYS[1], ARGV[1])
end
-- Non-JSON non-empty values are legacy transactionId strings; keep them.
if string.sub(val, 1, 1) ~= '{' then
  return 0
end
-- Keep keys that already recorded a provider/error outcome.
if string.match(val, '"transactionId"%s*:%s*"[^"]+"') then
  return 0
end
if string.match(val, '"errorMessage"%s*:%s*"[^"]+"') then
  return 0
end
return redis.call('HDEL', KEYS[1], ARGV[1])
`
	result, err := RDB.Eval(ctx, script, []string{commIdempotentKey}, redisKey).Int()
	if err != nil {
		return false, fmt.Errorf("failed to reclaim blank redis key %s: %w", redisKey, err)
	}
	if result > 0 {
		utils.Info(fmt.Sprintf("Reclaimed orphan blank redis key %s from hash %s", redisKey, commIdempotentKey))
		return true, nil
	}
	return false, nil
}

// 2. Update the value (e.g. responseId) for an existing mobile_channel key
// This function is kept for backward compatibility
func UpdateMobileChannelValue(RDB *redis.Client, commIdempotentKey, redisKey, responseId string) error {
	ctx := context.Background()
	err := RDB.HSet(ctx, commIdempotentKey, redisKey, responseId).Err()
	if err != nil {
		utils.Error(fmt.Errorf("failed to update value for key %s in redis: %v", redisKey, err))
		return err
	}
	utils.Info(fmt.Sprintf("Key %s in hash %s updated with value %s", redisKey, commIdempotentKey, responseId))
	return nil
}

// UpdateTransactionId updates the transactionId for an existing mobile_channel key
func UpdateTransactionId(RDB *redis.Client, commIdempotentKey, redisKey, transactionId string) error {
	ctx := context.Background()
	data, err := getMobileChannelRedisData(ctx, RDB, commIdempotentKey, redisKey)
	if err != nil {
		return err
	}
	data.TransactionId = transactionId
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}
	return RDB.HSet(ctx, commIdempotentKey, redisKey, string(jsonData)).Err()
}

func UpdatePushTransactionID(RDB *redis.Client, commIdempotentKey, redisKey, claimID, transactionId string) error {
	err := updatePushClaim(RDB, commIdempotentKey, redisKey, claimID, func(data *redisModels.MobileChannelRedisData) {
		data.TransactionId = transactionId
	})
	if err != nil {
		utils.Error(fmt.Errorf("failed to update transactionId for key %s in redis: %v", redisKey, err))
	}
	return err
}

func UpdatePushAttemptCount(RDB *redis.Client, commIdempotentKey, redisKey, claimID string, attemptCount int) error {
	if attemptCount < 0 {
		return fmt.Errorf("attempt count cannot be negative")
	}
	return updatePushClaim(RDB, commIdempotentKey, redisKey, claimID, func(data *redisModels.MobileChannelRedisData) {
		data.AttemptCount = attemptCount
	})
}

func UpdatePushErrorMessage(RDB *redis.Client, commIdempotentKey, redisKey, claimID, outcome, errorMessage string) error {
	return updatePushClaim(RDB, commIdempotentKey, redisKey, claimID, func(data *redisModels.MobileChannelRedisData) {
		data.Outcome = outcome
		data.ErrorMessage = errorMessage
	})
}

func RefreshPushClaim(RDB *redis.Client, commIdempotentKey, redisKey, claimID string) error {
	return updatePushClaim(RDB, commIdempotentKey, redisKey, claimID, func(data *redisModels.MobileChannelRedisData) {
		data.ClaimedAtUnix = time.Now().UTC().Unix()
	})
}

func updatePushClaim(rdb *redis.Client, hash, field, claimID string, update func(*redisModels.MobileChannelRedisData)) error {
	if rdb == nil {
		return fmt.Errorf("redis client is nil")
	}
	if strings.TrimSpace(claimID) == "" {
		return fmt.Errorf("PUSH claim id is required")
	}
	ctx := context.Background()
	return rdb.Watch(ctx, func(tx *redis.Tx) error {
		value, err := tx.HGet(ctx, hash, field).Result()
		if err != nil {
			return err
		}
		var data redisModels.MobileChannelRedisData
		if err := json.Unmarshal([]byte(value), &data); err != nil {
			return fmt.Errorf("invalid PUSH claim: %w", err)
		}
		if data.ClaimID != claimID {
			return fmt.Errorf("PUSH claim is no longer owned")
		}
		update(&data)
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal PUSH claim: %w", err)
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HSet(ctx, hash, field, string(jsonData))
			return nil
		})
		return err
	}, hash)
}

func getMobileChannelRedisData(ctx context.Context, rdb *redis.Client, hash, field string) (redisModels.MobileChannelRedisData, error) {
	if rdb == nil {
		return redisModels.MobileChannelRedisData{}, fmt.Errorf("redis client is nil")
	}
	value, err := rdb.HGet(ctx, hash, field).Result()
	if err != nil && err != redis.Nil {
		return redisModels.MobileChannelRedisData{}, fmt.Errorf("failed to get existing data for key %s: %v", field, err)
	}
	if value == "" {
		return redisModels.MobileChannelRedisData{}, nil
	}
	var data redisModels.MobileChannelRedisData
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		data.TransactionId = value
	}
	return data, nil
}

// UpdateErrorMessage updates the errorMessage for an existing mobile_channel key
func UpdateErrorMessage(RDB *redis.Client, commIdempotentKey, redisKey, errorMessage string) error {
	ctx := context.Background()
	if RDB == nil {
		return fmt.Errorf("redis client is nil")
	}

	// Get existing data
	val, err := RDB.HGet(ctx, commIdempotentKey, redisKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get existing data for key %s: %v", redisKey, err)
	}

	var data redisModels.MobileChannelRedisData
	if val != "" {
		// Try to parse existing JSON data
		if err := json.Unmarshal([]byte(val), &data); err != nil {
			// If it's not JSON, treat as old format and preserve as transactionId
			data.TransactionId = val
		}
	}

	// Update errorMessage
	data.ErrorMessage = errorMessage

	// Marshal back to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	err = RDB.HSet(ctx, commIdempotentKey, redisKey, string(jsonData)).Err()
	if err != nil {
		utils.Error(fmt.Errorf("failed to update errorMessage for key %s in redis: %v", redisKey, err))
		return err
	}
	utils.Info(fmt.Sprintf("Key %s in hash %s updated with errorMessage %s", redisKey, commIdempotentKey, errorMessage))
	return nil
}

// ClaimMarketingCampaignDedupKey atomically claims a campaign-level dedup slot (SET NX, no TTL). Key persists until daily FlushAll.
// WeCredit WA keys look like wecredit_whatsapp_{mobile}; other channels use mobile_client_process_channel.
// Returns true when the key was created, false when it already exists (duplicate).
func ClaimMarketingCampaignDedupKey(rdb *redis.Client, key, value string) (bool, error) {
	if rdb == nil {
		return false, fmt.Errorf("redis client is not initialized")
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return false, fmt.Errorf("campaign dedup key is required")
	}

	ctx := context.Background()
	ok, err := rdb.SetNX(ctx, key, value, 0).Result()

	if err != nil {
		return false, fmt.Errorf("campaign dedup claim: %w", err)
	}

	return ok, nil
}

// ReleaseMarketingCampaignDedupKey deletes a campaign dedup string key (rollback helper).
func ReleaseMarketingCampaignDedupKey(rdb *redis.Client, key string) error {
	if rdb == nil {
		return fmt.Errorf("redis client is not initialized")
	}

	ctx := context.Background()
	return rdb.Del(ctx, key).Err()
}

// ReleaseMobileChannelHashField removes one field from the idempotency hash (EventId rollback).
func ReleaseMobileChannelHashField(rdb *redis.Client, commIdempotentKey, field string) error {
	if rdb == nil {
		return fmt.Errorf("redis client is not initialized")
	}

	ctx := context.Background()
	return rdb.HDel(ctx, commIdempotentKey, field).Err()
}
