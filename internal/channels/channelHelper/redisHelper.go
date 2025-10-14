package channelHelper

import (
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// GenerateRedisKey creates a standardized Redis key for mobile_channel_stage
func GenerateRedisKey(mobile, channel string, stage float64) string {
	return fmt.Sprintf("%s_%s_%s", mobile, strings.ToUpper(channel), fmt.Sprintf("%.0f", stage))
}

// UpdateRedisTransactionId updates the transactionId in Redis with standardized error handling
func UpdateRedisTransactionId(mobile, channel string, stage float64, transactionId string) error {
	redisKey := GenerateRedisKey(mobile, channel, stage)
	err := redis.UpdateTransactionId(redis.RDB, config.Configs.CommIdempotentKey, redisKey, transactionId)
	if err != nil {
		utils.Error(fmt.Errorf("redis update for redisKey: %s transactionId: %s failed: %v", redisKey, transactionId, err))
		return err
	}
	return nil
}

// UpdateRedisErrorMessage updates the errorMessage in Redis with standardized error handling
func UpdateRedisErrorMessage(mobile, channel string, stage float64, errorMessage string) error {
	redisKey := GenerateRedisKey(mobile, channel, stage)
	err := redis.UpdateErrorMessage(redis.RDB, config.Configs.CommIdempotentKey, redisKey, errorMessage)
	if err != nil {
		utils.Error(fmt.Errorf("redis update for redisKey: %s errorMessage: %s failed: %v", redisKey, errorMessage, err))
		return err
	}
	return nil
}
