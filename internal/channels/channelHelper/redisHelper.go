package channelHelper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/models/redisModels"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// ErrWhatsappProcessRequired rejects a flagged WeCredit WhatsApp row that has no process.
var ErrWhatsappProcessRequired = errors.New("whatsapp process is required to skip the mobile check")

// ErrWhatsappAlreadySentForProcess rejects a second send of the same process to the same mobile.
var ErrWhatsappAlreadySentForProcess = errors.New("whatsapp already sent for this process")

// IsMarketingCampaignRequest is true for CommMarketingInput dispatch (EventId marketing-{id}).
func IsMarketingCampaignRequest(data sdkModels.CommApiRequestBody) bool {
	return strings.HasPrefix(strings.TrimSpace(data.EventId), "marketing-")
}

// GenerateMarketingCampaignDedupKey builds the standalone Redis string key for
// same-day marketing dedup (cleared by daily FlushAll at 1 AM IST).
//
// WeCredit WhatsApp uses client_channel_mobile — one WA send per number until
// flush, no process dimension (unlike SMS). Other channels keep
// mobile_client_process_channel (+ stage when non-zero).
func GenerateMarketingCampaignDedupKey(data sdkModels.CommApiRequestBody) string {
	mobile := strings.TrimSpace(data.Mobile)
	client := strings.ToLower(strings.TrimSpace(data.Client))
	channel := strings.ToUpper(strings.TrimSpace(data.Channel))

	if client == "wecredit" && channel == "WHATSAPP" {
		return fmt.Sprintf("%s_%s_%s", client, strings.ToLower(channel), mobile)
	}

	campaign := strings.ToLower(strings.TrimSpace(data.ProcessName))
	key := fmt.Sprintf("%s_%s_%s_%s", mobile, client, campaign, channel)

	if data.Stage != 0 {
		key += "_" + fmt.Sprintf("%.0f", data.Stage)
	}

	return key
}

// GenerateWhatsappProcessDedupKey is the flagged WeCredit WhatsApp key
// wecredit_whatsapp_{process}_{mobile}. The 1 AM IST FlushAll deletes every key,
// including this shape, so the claim sets no TTL.
func GenerateWhatsappProcessDedupKey(process, mobile string) string {
	return fmt.Sprintf("wecredit_whatsapp_%s_%s",
		strings.ToLower(strings.TrimSpace(process)),
		strings.TrimSpace(mobile))
}

// SkipsWeCreditWhatsappMobileDayCap is true for a WeCredit WhatsApp follow-up (IsFollowUpMessage = 1).
func SkipsWeCreditWhatsappMobileDayCap(data sdkModels.CommApiRequestBody) bool {
	return data.IsFollowUpMessage == 1 &&
		strings.EqualFold(strings.TrimSpace(data.Client), "wecredit") &&
		strings.EqualFold(strings.TrimSpace(data.Channel), "WHATSAPP")
}

// CampaignDuplicateMessage is the terminal duplicate text for a marketing send.
// Flagged WeCredit WhatsApp uses a process message. Unflagged WeCredit WhatsApp
// stays "whatsapp already sent today".
func CampaignDuplicateMessage(data sdkModels.CommApiRequestBody) string {
	client := strings.ToLower(strings.TrimSpace(data.Client))
	channel := strings.ToUpper(strings.TrimSpace(data.Channel))
	if SkipsWeCreditWhatsappMobileDayCap(data) {
		process := strings.ToLower(strings.TrimSpace(data.ProcessName))
		if process == "" {
			return ErrWhatsappProcessRequired.Error()
		}

		return fmt.Sprintf("whatsapp already sent for this process %s mobile %s", process, strings.TrimSpace(data.Mobile))
	}

	if client == "wecredit" && channel == "WHATSAPP" {
		return fmt.Sprintf("whatsapp already sent today for mobile %s", strings.TrimSpace(data.Mobile))

	}
	return fmt.Sprintf("campaign duplicate: channel %s process %s event_id %s already sent today",
		channel,
		strings.ToLower(strings.TrimSpace(data.ProcessName)),
		strings.TrimSpace(data.EventId))
}

// FlaggedWhatsappClaim records which day-cap keys this attempt created.
type FlaggedWhatsappClaim struct {
	ProcessOwned bool
	MobileOwned  bool
}

// ClaimFlaggedWeCreditWhatsapp claims wecredit_whatsapp_{process}_{mobile}, then
// tries the plain mobile key. A lost mobile SET NX is ignored. A Redis error
// frees the process key this attempt claimed and is returned for retry.
func ClaimFlaggedWeCreditWhatsapp(rdb *goredis.Client, data sdkModels.CommApiRequestBody) (FlaggedWhatsappClaim, error) {
	if strings.TrimSpace(data.ProcessName) == "" {
		return FlaggedWhatsappClaim{}, ErrWhatsappProcessRequired
	}

	eventID := strings.TrimSpace(data.EventId)
	processKey := GenerateWhatsappProcessDedupKey(data.ProcessName, data.Mobile)
	owned, err := redis.ClaimMarketingCampaignDedupKey(rdb, processKey, eventID)
	if err != nil {
		return FlaggedWhatsappClaim{}, err
	}

	var claim FlaggedWhatsappClaim
	if owned || campaignDedupValueMatches(rdb, processKey, eventID) {
		claim.ProcessOwned = true
	} else {
		return FlaggedWhatsappClaim{}, ErrWhatsappAlreadySentForProcess
	}

	mobileKey := GenerateMarketingCampaignDedupKey(data)
	mobileOwned, err := redis.ClaimMarketingCampaignDedupKey(rdb, mobileKey, eventID)
	if err != nil {
		if claim.ProcessOwned {
			_ = redis.ReleaseMarketingCampaignDedupKeyIfValue(rdb, processKey, eventID)
		}
		return FlaggedWhatsappClaim{}, err
	}

	claim.MobileOwned = mobileOwned || campaignDedupValueMatches(rdb, mobileKey, eventID)
	return claim, nil
}

// ReleaseFlaggedWeCreditWhatsapp frees only keys whose value is this event id.
// A mobile key owned by another sender is left in place.
func ReleaseFlaggedWeCreditWhatsapp(rdb *goredis.Client, data sdkModels.CommApiRequestBody) {
	eventID := strings.TrimSpace(data.EventId)
	_ = redis.ReleaseMarketingCampaignDedupKeyIfValue(rdb, GenerateWhatsappProcessDedupKey(data.ProcessName, data.Mobile), eventID)
	_ = redis.ReleaseMarketingCampaignDedupKeyIfValue(rdb, GenerateMarketingCampaignDedupKey(data), eventID)
}

func campaignDedupValueMatches(rdb *goredis.Client, key, eventID string) bool {
	if rdb == nil || eventID == "" {
		return false
	}

	existing, err := rdb.Get(context.Background(), key).Result()
	return err == nil && strings.TrimSpace(existing) == eventID
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

// SealRedisTerminalResult records the provider-attempt timestamp and provider
// identity in the existing idempotency field. It does not alter key lifetime.
func SealRedisTerminalResult(msg sdkModels.CommApiRequestBody, transactionID, providerMessageID, outcome, errorMessage string, hitAt time.Time) error {
	redisKey := GenerateRedisKeyForRequest(msg)
	err := redis.SealTerminalResult(redis.RDB, config.Configs.CommIdempotentKey, redisKey, transactionID, providerMessageID, outcome, errorMessage, hitAt)
	if err != nil {
		utils.Error(fmt.Errorf("redis terminal seal for redisKey %s failed: %v", redisKey, err))
		return err
	}

	return nil
}

// GetRedisTerminalResult reads the existing terminal envelope. Legacy scalar
// values are returned without HitAt so the persistence lane can stamp the same
// completion time the previous synchronous insert would have used.
func GetRedisTerminalResult(msg sdkModels.CommApiRequestBody) (redisModels.MobileChannelRedisData, error) {
	redisKey := GenerateRedisKeyForRequest(msg)
	return redis.GetTerminalResult(redis.RDB, config.Configs.CommIdempotentKey, redisKey)
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
