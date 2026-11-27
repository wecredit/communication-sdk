package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/internal/channels/whatsapp"
	"github.com/wecredit/communication-sdk/internal/database"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	sdkServices "github.com/wecredit/communication-sdk/sdk/services"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func handleQueryCampaignWhatsapp(ctx context.Context, data sdkModels.CommApiRequestBody, dbMappedData map[string]interface{}, sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) (bool, bool) {
	data.CommId = sdkServices.ResolveCommID(data.CommId, data.Client)

	claimed, err := database.ClaimQueryCampaignSending(data.Client, data.SourceRowId)
	if err != nil {
		utils.Error(fmt.Errorf("[Client:%s SourceRowId:%d] query campaign WA SENDING claim failed: %v", data.Client, data.SourceRowId, err))
		return false, false
	}
	if !claimed {
		deleted, delErr := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		if !deleted {
			utils.Error(fmt.Errorf("failed to delete redelivered query campaign WhatsApp: %v", delErr))
		}
		return true, deleted
	}

	if !channelHelper.ShouldHitVendor(data.Client, data.Channel) {
		finalizeQueryCampaignFromRequest(data, sdkHelper.LedgerNotHit, sdkHelper.LedgerNotHit, "", "shouldHitVendor is off", false)
		deleted, _ := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		return true, deleted
	}

	if !AssignVendor(&data) {
		finalizeQueryCampaignFromRequest(data, sdkHelper.LedgerFailed, "FAILED", "", "requested vendor is not active", false)
		deleted, _ := deleteMessage(ctx, sqsClient, queueURL, msg, data)
		return true, deleted
	}

	dbMappedData["CommId"] = data.CommId
	if strings.TrimSpace(config.Configs.SdkWhatsappInputTable) != "" {
		database.InsertRow(data.Client, config.Configs.SdkWhatsappInputTable, dbMappedData)
	}

	wpResult, sendErr := whatsapp.SendWpByProcess(data)
	terminal := sdkHelper.LedgerFailed
	outcome := "FAILED"
	isSent := false
	if wpResult.Accepted {
		terminal = sdkHelper.LedgerSent
		outcome = "SENT"
		isSent = true
	}
	txn := strings.TrimSpace(wpResult.TransactionID)
	respMsg := ""
	if sendErr != nil {
		respMsg = sendErr.Error()
	}
	finalizeQueryCampaignFromRequest(data, terminal, outcome, txn, respMsg, isSent)

	if strings.TrimSpace(config.Configs.WhatsappOutputTable) != "" && wpResult.DBData != nil {
		database.InsertRow(data.Client, config.Configs.WhatsappOutputTable, MapWhatsappMysqlOutput(wpResult.DBData))
	}

	if !wpResult.Processed && sendErr != nil {
		utils.Error(fmt.Errorf("[Client:%s SourceRowId:%d] query campaign WhatsApp non-terminal; leaving SENDING: %v", data.Client, data.SourceRowId, sendErr))
		return false, false
	}

	deleted, delErr := deleteMessage(ctx, sqsClient, queueURL, msg, data)
	if !deleted {
		utils.Error(fmt.Errorf("failed to delete query campaign WhatsApp after send: %v", delErr))
	}
	return true, deleted
}
