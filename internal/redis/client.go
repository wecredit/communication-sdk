package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

var (
	RDB *redis.Client
)

// RedisClient initializes the Redis connection
func RedisClient(address, password string) error {
	if address == "" {
		return fmt.Errorf("environment variables for Redis are not set")
	}

	// Initialize Redis client
	RDB = redis.NewClient(&redis.Options{
		Addr:      address,
		DB:        0,                                     // Default database
		TLSConfig: &tls.Config{InsecureSkipVerify: true}, // Set false in production with valid certs
	})

	// Test the Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RDB.Ping(ctx).Result()
	if err != nil {
		utils.Error(fmt.Errorf("failed to connect to Redis: %v", err))
		return err
	}

	utils.Info("Redis connection established.")
	return nil
}

// GetRedisClient retrieves the Redis client, initializing it if not already done
func GetRedisClient(address, password string) (*redis.Client, error) {
	if RDB == nil {
		if err := RedisClient(address, password); err != nil {
			utils.Error(fmt.Errorf("failed to initialize Redis client: %v", err))
			return nil, err
		}
	}
	return RDB, nil
}

// CloseRedisClient gracefully closes the Redis client
func CloseRedisClient() {
	if RDB != nil {
		err := RDB.Close()
		if err != nil {
			utils.Error(fmt.Errorf("failed to close Redis client: %v", err))
		} else {
			utils.Info("Redis connection closed.")
		}
	}
}
