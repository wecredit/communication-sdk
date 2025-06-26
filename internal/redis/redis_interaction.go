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
