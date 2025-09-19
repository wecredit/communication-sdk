package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

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

// Check if mobile_channel exists and return value if present
func CheckIfMobileExists(CommIdempotentKey string, redisKey string, rdb *redis.Client) (string, bool, error) {
	ctx := context.Background()
	val, err := rdb.HGet(ctx, CommIdempotentKey, redisKey).Result()
	if err == redis.Nil {
		utils.Info(fmt.Sprintf("[redis]: %s does not exist Proceed for communication", redisKey))
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// 1. Create a field (mobile_channel) inside CommIdempotentKey with blank value
func SetMobileChannelKey(RDB *redis.Client, commIdempotentKey, redisKey string) error {
	ctx := context.Background()
	err := RDB.HSetNX(ctx, commIdempotentKey, redisKey, "").Err()
	if err != nil {
		utils.Error(fmt.Errorf("failed to set key %s in redis: %v", redisKey, err))
		return err
	}
	utils.Info(fmt.Sprintf("Key %s created in hash %s with blank value", redisKey, commIdempotentKey))
	return nil
}

// 2. Update the value (e.g. responseId) for an existing mobile_channel key
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
