package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/sms"
	"github.com/wecredit/communication-sdk/internal/database"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	sdkServices "github.com/wecredit/communication-sdk/sdk/services"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// handleQueryCampaignSMS claims SENDING, respects Clients.ShouldHitVendor (NOT_HIT when off),
// sends, then always finalizes ledger terminal + output before ACK.
func handleQueryCampaignSMS(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	data.CommId = sdkServices.ResolveCommID(data.CommId, data.Client)

	claimed, err := database.ClaimQueryCampaignSending(data.Client, data.SourceRowId)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s SourceRowId:%d] query campaign SENDING claim failed: %v", data.Client, data.SourceRowId, err))
		return false, false
	}
	if !claimed {
		deleted, delErr := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete redelivered query campaign SMS: %v", delErr))
		}
		return true, deleted
	}

	if !channelHelper.ShouldHitVendor(data.Client, data.Channel) {
		finalizeQueryCampaignFromRequest(data, sdkHelper.LedgerNotHit, sdkHelper.LedgerNotHit, "", "shouldHitVendor is off", false)
		deleted, delErr := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete NOT_HIT query campaign SMS: %v", delErr))
		}
		return true, deleted
	}

	if !AssignVendor(&data) {
		finalizeQueryCampaignFromRequest(data, sdkHelper.LedgerFailed, "FAILED", "", "requested vendor is not active", false)
		deleted, _ := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		return true, deleted
	}

	dbMappedData["CommId"] = data.CommId
	if strings.TrimSpace(config.Configs.SdkSmsInputTable) != "" {
		database.InsertRow(data.Client, config.Configs.SdkSmsInputTable, dbMappedData)
	}

	result, sendErr := sms.SendSmsByProcessWithContext(ctx, data)
	terminal := sdkHelper.LedgerFailed
	outcome := "FAILED"
	isSent := false
	if result.Accepted {
		terminal = sdkHelper.LedgerSent
		outcome = "SENT"
		isSent = true
	}
	txn := strings.TrimSpace(result.TransactionID)
	respMsg := ""
	if sendErr != nil {
		respMsg = sendErr.Error()
	} else if result.DBData != nil {
		if v, ok := result.DBData["ResponseMessage"].(string); ok {
			respMsg = v
		}
	}
	finalizeQueryCampaignFromRequest(data, terminal, outcome, txn, respMsg, isSent)

	if strings.TrimSpace(config.Configs.SmsOutputTable) != "" && result.DBData != nil {
		database.InsertRow(data.Client, config.Configs.SmsOutputTable, result.DBData)
	}

	if !result.AckSQS && sendErr != nil && !result.Processed {
		// Non-terminal provider outcome — leave SENDING for grace/expiry; do not ACK.
		utils.Error(fmt.Errorf("[Client:%s SourceRowId:%d] query campaign SMS non-terminal; leaving SENDING: %v", data.Client, data.SourceRowId, sendErr))
		return false, false
	}

	deleted, delErr := deleteMessage(ctx, sqsClient, queueURL, msg, data)
	if !deleted {
		utils.Error(fmt.Errorf("failed to delete query campaign SMS after send: %v", delErr))
	}
	return true, deleted
}
