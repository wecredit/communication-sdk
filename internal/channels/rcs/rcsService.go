package rcs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	pinnacleRcs "github.com/wecredit/communication-sdk/internal/channels/rcs/pinnacle"
	sinchRcs "github.com/wecredit/communication-sdk/internal/channels/rcs/sinch"
	timesRcs "github.com/wecredit/communication-sdk/internal/channels/rcs/times"
	"github.com/wecredit/communication-sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	dbservices "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

type SendRcsResult struct {
	Processed            bool
	Accepted             bool
	ResolvedVendor       string
	ResolvedTemplate     string
	TemplateVariables    string
	SMSFallbackVariables string
	TransactionID        string
}

func ApplyRcsResponseDefaults(response *extapimodels.RcsResponse, msg sdkModels.CommApiRequestBody, shouldHitVendor bool, templateName string) {
	response.CommId = msg.CommId
	response.TemplateName = templateName
	response.Vendor = msg.Vendor
	response.MobileNumber = msg.Mobile
	if !shouldHitVendor && strings.TrimSpace(response.ResponseMessage) == "" {
		response.ResponseMessage = "shouldHitVendor is off for mobile " + msg.Mobile
		return
	}

	if strings.TrimSpace(response.ResponseMessage) == "" && !response.IsSent {
		response.ResponseMessage = "RCS provider returned no response message for mobile " + msg.Mobile
	}
}

func SendRcsByProcess(msg sdkModels.CommApiRequestBody) (SendRcsResult, error) {
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		return SendRcsResult{}, errors.New("template data not found in cache")
	}
	templateData, matchedVendor, err := channelHelper.ResolveTemplateData(msg, templateDetails)
	if err != nil {
		channelHelper.LogTemplateNotFound(msg, err)
		return SendRcsResult{Processed: false}, nil
	}
	msg.Vendor = matchedVendor

	req := extapimodels.RcsRequestBody{
		Mobile:             msg.Mobile,
		Process:            msg.ProcessName,
		Client:             msg.Client,
		CommId:             msg.CommId,
		EmiAmount:          msg.EmiAmount,
		CustomerName:       msg.CustomerName,
		LoanId:             msg.LoanId,
		ApplicationNumber:  msg.ApplicationNumber,
		DueDate:            msg.DueDate,
		Description:        msg.Description,
		TotalPayableAmount: msg.TotalPayableAmount,
		TodayPayableAmount: msg.TodayPayableAmount,
		SavingAmount:       msg.SavingAmount,
		BounceCharge:       msg.BounceCharge,
		PaymentLink:        msg.PaymentLink,
	}
	channelHelper.PopulateRcsFields(&req, templateData)

	if msg.Vendor == variables.SINCH {
		rcsAppIdData, err := database.GetRcsAppId(database.DBtechRead, req.AppId)
		if err != nil {
			utils.Error(fmt.Errorf("failed to fetch RCS AppId data: %v", err))
			return SendRcsResult{}, fmt.Errorf("failed to fetch RCS AppId data: %v", err)
		}
		if val, ok := rcsAppIdData["AppIdKey"].(string); ok {
			req.AppIdKey = val
		}
		if val, ok := rcsAppIdData["ProjectId"].(string); ok {
			req.ProjectId = val
			req.ApiKey = val
		}
	}

	var response extapimodels.RcsResponse
	// Check if the vendor should be hit
	shouldHitVendor := channelHelper.ShouldHitVendor(msg.Client, msg.Channel)

	if shouldHitVendor {
		switch msg.Vendor {
		case variables.TIMES:
			response = timesRcs.HitTimesRcsApi(req)
		case variables.SINCH:
			response = sinchRcs.HitSinchRcsApi(req)
		case variables.PINNACLE:
			response = pinnacleRcs.HitPinnacleRcsApi(req)
		}
	}

	ApplyRcsResponseDefaults(&response, msg, shouldHitVendor, req.TemplateName)

	dbMappedData, err := dbservices.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("mapping error: %v", err))
	}

	if err := database.InsertData(config.Configs.RcsOutputTable, database.DBtechWrite, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting RCS output for CommId %s: %v", msg.CommId, err))
		return SendRcsResult{Processed: false}, fmt.Errorf("error inserting RCS output for CommId %s: %w", msg.CommId, err)
	}

	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("RCS Response: %s", string(jsonBytes)))

	result := SendRcsResult{
		Processed:            true,
		Accepted:             response.IsSent,
		ResolvedVendor:       msg.Vendor,
		ResolvedTemplate:     req.TemplateName,
		TemplateVariables:    req.TemplateVariables,
		SMSFallbackVariables: req.SmsFallbackVariables,
		TransactionID:        response.TransactionId,
	}

	if response.IsSent {
		utils.Info(fmt.Sprintf("RCS sent successfully for Process: %s on %s via %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return result, nil
	}

	return result, nil
}
