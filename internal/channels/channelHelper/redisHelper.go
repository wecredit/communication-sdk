package channelHelper

import (
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// IsMarketingCampaignRequest is true for CommMarketingInput dispatch (EventId marketing-{id}).
func IsMarketingCampaignRequest(data sdkModels.CommApiRequestBody) bool {
	return strings.HasPrefix(strings.TrimSpace(data.EventId), "marketing-")
}

// GenerateMarketingCampaignDedupKey builds the standalone Redis string key for
// same-day mobile+campaign+channel dedup (cleared by daily FlushAll). Stage omitted when Stage is 0.
func GenerateMarketingCampaignDedupKey(data sdkModels.CommApiRequestBody) string {
	mobile := strings.TrimSpace(data.Mobile)
	campaign := strings.ToLower(strings.TrimSpace(data.ProcessName))
	channel := strings.ToUpper(strings.TrimSpace(data.Channel))
	key := fmt.Sprintf("%s_%s_%s", mobile, campaign, channel)

	if data.Stage != 0 {
		key += "_" + fmt.Sprintf("%.0f", data.Stage)
	}
	
	return key
}

// GenerateRedisKey creates the legacy Redis field: mobile_CHANNEL_stageInt.
// Prefer GenerateRedisKeyForRequest for new call sites.
func GenerateRedisKey(mobile, channel string, stage float64) string {
	return fmt.Sprintf("%s_%s_%s", mobile, strings.ToUpper(channel), fmt.Sprintf("%.0f", stage))
}

// GenerateRedisKeyForRequest builds the Redis idempotency field for a send attempt.
//
// Priority:
//  1. EventId (marketing dispatch durable identity) — unique per CommMarketingInput row
//  2. TemplateReference + process + mobile — template-keyed sends without EventId
//  3. Legacy mobile_CHANNEL_stageInt — ZapCash / CreditSea nurture journeys
//
// CommId is intentionally not used here: it follows WC-<CLIENT>-UUID-ts like other lenders.
func GenerateRedisKeyForRequest(data sdkModels.CommApiRequestBody) string {
	if id := strings.TrimSpace(data.EventId); id != "" {
		return id
	}
	if ref := strings.TrimSpace(data.TemplateReference); ref != "" {
		process := strings.ToLower(strings.TrimSpace(data.ProcessName))
		return fmt.Sprintf("%s_%s_%s_%s",
			strings.TrimSpace(data.Mobile),
			strings.ToUpper(strings.TrimSpace(data.Channel)),
			process,
			strings.ToLower(ref),
		)
	}
	return GenerateRedisKey(data.Mobile, data.Channel, data.Stage)
}

// UpdateRedisTransactionId updates the transactionId in Redis with standardized error handling.
func UpdateRedisTransactionId(msg sdkModels.CommApiRequestBody, transactionId string) error {
	redisKey := GenerateRedisKeyForRequest(msg)
	err := redis.UpdateTransactionId(redis.RDB, config.Configs.CommIdempotentKey, redisKey, transactionId)
	if err != nil {
		utils.Error(fmt.Errorf("redis update for redisKey: %s transactionId: %s failed: %v", redisKey, transactionId, err))
		return err
	}
	return nil
}

// UpdateRedisErrorMessage updates the errorMessage in Redis with standardized error handling.
func UpdateRedisErrorMessage(msg sdkModels.CommApiRequestBody, errorMessage string) error {
	redisKey := GenerateRedisKeyForRequest(msg)
	err := redis.UpdateErrorMessage(redis.RDB, config.Configs.CommIdempotentKey, redisKey, errorMessage)
	if err != nil {
		utils.Error(fmt.Errorf("redis update for redisKey: %s errorMessage: %s failed: %v", redisKey, errorMessage, err))
		return err
	}
	return nil
}
