package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/internal/database"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func isQueryCampaignDispatch(data sdkModels.CommApiRequestBody) bool {
	return strings.EqualFold(strings.TrimSpace(data.Source), sdkHelper.SourceQueryCampaign) && data.SourceRowId != 0
}

func finalizeQueryCampaignFromRequest(data sdkModels.CommApiRequestBody, terminal, outcome, txn, response string, isSent bool) {
	campaignID, runID, _ := database.LookupQueryCampaignLedgerMeta(data.Client, data.SourceRowId)
	now := time.Now().UTC()
	hitAt := &now
	row := database.QueryCampaignLedgerOutputRow{
		LedgerInputId:     data.SourceRowId,
		CampaignId:        campaignID,
		RunId:             runID,
		CommId:            data.CommId,
		EventId:           data.EventId,
		Client:            data.Client,
		Channel:           data.Channel,
		Vendor:            data.Vendor,
		ProcessName:       data.ProcessName,
		Mobile:            data.Mobile,
		TemplateReference: data.TemplateReference,
		IsSent:            isSent,
		Outcome:           outcome,
		TransactionId:     txn,
		ResponseMessage:   truncateQCMsg(response, 1024),
		HitAt:             hitAt,
	}
	if err := database.FinalizeQueryCampaignLedger(row, terminal); err != nil {
		utils.Error(fmt.Errorf("query campaign ledger finalize failed: %v", err))
	}
}

func truncateQCMsg(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
