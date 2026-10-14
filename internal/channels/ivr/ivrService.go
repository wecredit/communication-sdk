package ivr

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	pinnacleIvr "github.com/wecredit/communication-sdk/internal/channels/ivr/pinnacle"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

const (
	markerAttempted = "ivr_attempted"
	markerRejected  = "ivr_provider_rejected"
	markerUnknown   = "ivr_outcome_unknown"
	markerNotSent   = "ivr_not_sent"
)

var (
	voiceIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
	msisdnPattern  = regexp.MustCompile(`^[0-9]{10}$`)

	stampClaimFn = func(hash, field, marker string) (int, error) {
		return redis.StampIvrClaimIfBlank(redis.RDB, hash, field, marker)
	}
	setOutcomeFn = func(hash, field, transactionID, category string) error {
		return redis.SetIvrClaimOutcome(redis.RDB, hash, field, transactionID, category)
	}
	reclaimBlankFn = func(hash, field string) (bool, error) {
		return redis.ReclaimBlankMobileChannelKey(redis.RDB, hash, field)
	}
)

// SetClaimHooksForTest replaces the Redis claim calls for one test.
func SetClaimHooksForTest(
	stamp func(hash, field, marker string) (int, error),
	outcome func(hash, field, transactionID, category string) error,
	reclaim func(hash, field string) (bool, error),
) func() {
	previousStamp, previousOutcome, previousReclaim := stampClaimFn, setOutcomeFn, reclaimBlankFn
	if stamp != nil {
		stampClaimFn = stamp
	}

	if outcome != nil {
		setOutcomeFn = outcome
	}

	if reclaim != nil {
		reclaimBlankFn = reclaim
	}

	return func() {
		stampClaimFn, setOutcomeFn, reclaimBlankFn = previousStamp, previousOutcome, previousReclaim
	}
}

// AttemptResult tells the consumer whether the SQS receipt should be deleted.
// DeleteMessage false leaves the message until visibility ends.
type AttemptResult struct {
	DeleteMessage  bool
	InactiveVendor bool
}

// SendAttempt places at most one Pinnacle call for an established stamp.
func SendAttempt(ctx context.Context, data *sdkModels.CommApiRequestBody, inputAudit map[string]interface{}) AttemptResult {
	if data == nil {
		return AttemptResult{DeleteMessage: true}
	}

	if !channelHelper.IsVendorActive(data.Client, data.Vendor, data.Channel) {
		return AttemptResult{InactiveVendor: true}
	}

	voiceID, templateName, reject := resolveSend(*data)
	if reject != "" {
		return finishNotSent(*data, inputAudit, templateName, reject)
	}

	if !channelHelper.ShouldHitVendor(data.Client, data.Channel) {
		return finishNotSent(*data, inputAudit, templateName, "should_hit_vendor_off")
	}

	if ctx.Err() != nil {
		return cancelBeforeStamp(*data)
	}

	if err := ratelimit.WaitFor(ctx, ratelimit.KeyWithChannel(variables.PINNACLE, data.Client, "ivr")); err != nil || ctx.Err() != nil {
		return cancelBeforeStamp(*data)
	}

	redisKey := channelHelper.GenerateRedisKeyForRequest(*data)
	stamp, stampErr := stampClaimFn(config.Configs.CommIdempotentKey, redisKey, markerAttempted)
	if stampErr != nil {
		utils.Error(fmt.Errorf("ivr_attempt_stamp_ambiguous commId=%s", data.CommId))
		return AttemptResult{}
	}

	switch stamp {
	case 1:
	case 0:
		utils.Info(fmt.Sprintf("ivr_attempt_already_marked commId=%s", data.CommId))
		return AttemptResult{DeleteMessage: true}
	default:
		utils.Error(fmt.Errorf("ivr_attempt_stamp_failed commId=%s", data.CommId))
		return AttemptResult{}
	}

	if ctx.Err() != nil {
		recordOutcome(*data, "", markerUnknown)
		writeAudits(*data, inputAudit, templateName, false, pinnacleIvr.PrefixUnknown+" cancelled", "", "")
		return AttemptResult{DeleteMessage: true}
	}

	result, callErr := pinnacleIvr.PostSingleCall(ctx, config.Configs.PinnacleZapcashIvrApiUrl, pinnacleIvr.CallRequest{
		UserName:   config.Configs.PinnacleZapcashIvrApiUsername,
		Password:   config.Configs.PinnacleZapcashIvrApiPassword,
		VoiceId:    voiceID,
		MSISDN:     strings.TrimSpace(data.Mobile),
		CustCallID: data.CommId,
		OBDType:    "SINGLE_VOICE",
	})

	if callErr != nil && result.RedisCategory == "" {
		result.RedisCategory = markerUnknown
		result.ResponseMessage = pinnacleIvr.PrefixUnknown
	}

	if result.IsSent {
		if result.TransactionID != "" {
			recordOutcome(*data, result.TransactionID, "")
		}

		utils.Info(fmt.Sprintf("provider_accepted commId=%s client=%s http=%d", data.CommId, data.Client, result.HTTPStatus))
	} else {
		category := result.RedisCategory
		if category == "" {
			category = markerUnknown
		}

		recordOutcome(*data, "", category)
		logCategory := "provider_outcome_unknown"
		if category == markerRejected {
			logCategory = "provider_rejected"
		}

		utils.Info(fmt.Sprintf("%s commId=%s client=%s http=%d", logCategory, data.CommId, data.Client, result.HTTPStatus))
	}

	writeAudits(*data, inputAudit, templateName, result.IsSent, result.ResponseMessage, result.TransactionID, result.CampaignID)
	return AttemptResult{DeleteMessage: true}
}

func resolveSend(data sdkModels.CommApiRequestBody) (voiceID, templateName, reject string) {
	if strings.TrimSpace(config.Configs.PinnacleZapcashIvrApiUrl) == "" ||
		strings.TrimSpace(config.Configs.PinnacleZapcashIvrApiUsername) == "" ||
		strings.TrimSpace(config.Configs.PinnacleZapcashIvrApiPassword) == "" ||
		strings.Contains(config.Configs.PinnacleZapcashIvrApiUrl, "@") {
		return "", "", "config"
	}

	if !msisdnPattern.MatchString(strings.TrimSpace(data.Mobile)) {
		return "", "", "msisdn"
	}

	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		return "", "", "template"
	}

	templateData, _, err := channelHelper.ResolveTemplateData(data, templateDetails)
	if err != nil {
		return "", "", "template"
	}

	name, _ := templateData["TemplateName"].(string)
	voice, _ := templateData["VoiceId"].(string)
	voice = strings.TrimSpace(voice)
	if !voiceIDPattern.MatchString(voice) {
		return "", strings.TrimSpace(name), "voice_id"
	}

	return voice, strings.TrimSpace(name), ""
}

func finishNotSent(data sdkModels.CommApiRequestBody, inputAudit map[string]interface{}, templateName, category string) AttemptResult {
	redisKey := channelHelper.GenerateRedisKeyForRequest(data)
	if _, err := stampClaimFn(config.Configs.CommIdempotentKey, redisKey, markerNotSent); err != nil {
		utils.Error(fmt.Errorf("ivr_attempt_stamp_ambiguous commId=%s category=%s", data.CommId, category))
	}

	utils.Info(fmt.Sprintf("ivr_not_sent commId=%s category=%s", data.CommId, category))
	writeAudits(data, inputAudit, templateName, false, pinnacleIvr.PrefixNotSent+" "+category, "", "")
	return AttemptResult{DeleteMessage: true}
}

func cancelBeforeStamp(data sdkModels.CommApiRequestBody) AttemptResult {
	redisKey := channelHelper.GenerateRedisKeyForRequest(data)
	if _, err := reclaimBlankFn(config.Configs.CommIdempotentKey, redisKey); err != nil {
		utils.Error(fmt.Errorf("ivr_rate_limit_release_failed commId=%s", data.CommId))
		return AttemptResult{}
	}

	utils.Info(fmt.Sprintf("ivr_rate_limit_cancelled commId=%s", data.CommId))
	return AttemptResult{}
}

func recordOutcome(data sdkModels.CommApiRequestBody, transactionID, category string) {
	redisKey := channelHelper.GenerateRedisKeyForRequest(data)
	if err := setOutcomeFn(config.Configs.CommIdempotentKey, redisKey, transactionID, category); err != nil {
		utils.Error(fmt.Errorf("ivr_outcome_update_failed commId=%s category=%s", data.CommId, category))
	}
}

func writeAudits(data sdkModels.CommApiRequestBody, inputAudit map[string]interface{}, templateName string, isSent bool, responseMessage, transactionID, campaignID string) {
	if inputAudit == nil {
		inputAudit = map[string]interface{}{}
	}

	inputAudit["CommId"] = data.CommId
	logAuditEnqueue(data.CommId, config.Configs.SdkIvrInputTable)
	database.InsertRow(data.Client, config.Configs.SdkIvrInputTable, inputAudit)

	output := map[string]interface{}{
		"CommId":          data.CommId,
		"TemplateName":    templateName,
		"TransactionId":   transactionID,
		"Vendor":          data.Vendor,
		"IsSent":          isSent,
		"ResponseMessage": responseMessage,
		"MobileNumber":    data.Mobile,
	}
	if campaignID != "" {
		output["CampaignId"] = campaignID
	}

	logAuditEnqueue(data.CommId, config.Configs.IvrOutputTable)
	database.InsertRow(data.Client, config.Configs.IvrOutputTable, output)
}

func logAuditEnqueue(commID, table string) {
	utils.Info(fmt.Sprintf("ivr_audit_enqueue commId=%s table=%s", commID, table))
}
