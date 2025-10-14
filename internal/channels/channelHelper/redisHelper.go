package channelHelper

import (
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// GenerateRedisKey creates a standardized Redis key for mobile_channel
func GenerateRedisKey(mobile, channel string) string {
	return fmt.Sprintf("%s_%s", mobile, strings.ToUpper(channel))
}

// UpdateRedisTransactionId updates the transactionId in Redis with standardized error handling
func UpdateRedisTransactionId(mobile, channel, transactionId string) error {
	redisKey := GenerateRedisKey(mobile, channel)
	err := redis.UpdateTransactionId(redis.RDB, config.Configs.CommIdempotentKey, redisKey, transactionId)
	if err != nil {
		utils.Error(fmt.Errorf("redis update for redisKey: %s transactionId: %s failed: %v", redisKey, transactionId, err))
		return err
	}
	return nil
}

// UpdateRedisErrorMessage updates the errorMessage in Redis with standardized error handling
func UpdateRedisErrorMessage(mobile, channel, errorMessage string) error {
	redisKey := GenerateRedisKey(mobile, channel)
	err := redis.UpdateErrorMessage(redis.RDB, config.Configs.CommIdempotentKey, redisKey, errorMessage)
	if err != nil {
		utils.Error(fmt.Errorf("redis update for redisKey: %s errorMessage: %s failed: %v", redisKey, errorMessage, err))
		return err
	}
	return nil
}
