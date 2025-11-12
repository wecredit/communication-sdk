package email

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	sinchEmail "github.com/wecredit/communication-sdk/internal/channels/email/sinch"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendEmailByProcess(msg sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {

	requestBody := extapimodels.EmailRequestBody{
		ToEmail:           msg.Email,
		Process:           msg.ProcessName,
		Client:            msg.Client,
		EmiAmount:         msg.EmiAmount,
		CustomerName:      msg.CustomerName,
		LoanId:            msg.LoanId,
		ApplicationNumber: msg.ApplicationNumber,
		DueDate:           msg.DueDate,
		Description:       msg.Description,
	}

	utils.Debug("Fetching Email process data from cache")
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return false, nil, errors.New("template data not found in cache")
	}
	data, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		return channelHelper.HandleTemplateNotFoundError(msg, err)
	}
	msg.Vendor = matchedVendor

	channelHelper.PopulateEmailFields(&requestBody, data)

	var response extapimodels.EmailResponse
	// Check if the vendor should be hit
	shouldHitVendor := channelHelper.ShouldHitVendor(msg.Client, msg.Channel)

	if shouldHitVendor {
		// Hit Into Email
		switch msg.Vendor {
		case variables.TIMES:
			return false, nil, errors.New("times email is not supported yet")
		case variables.SINCH:
			response = sinchEmail.HitSinchEmailApi(requestBody)
		}
	}

	// Step 2: Once you have responseId, update the value of transactionId in redis
	if err := channelHelper.UpdateRedisTransactionId(msg.Mobile, msg.Channel, msg.Stage, response.TransactionId); err != nil {
		utils.Error(fmt.Errorf("failed to update Redis transactionId: %v", err))
	}

	response.TemplateName = requestBody.TemplateId
	response.CommId = msg.CommId
	response.Vendor = msg.Vendor
	response.Email = msg.Email

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	// if err := database.InsertData(config.Configs.EmailOutputTable, database.DBtech, dbMappedData); err != nil {
	// 	utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	// }

	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("EmailResponse: %s", string(jsonBytes)))
	if shouldHitVendor && response.IsSent {
		utils.Info(fmt.Sprintf("Email sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Email, msg.Vendor))
		return true, dbMappedData, nil
	}

	if !shouldHitVendor {
		// Step 2: Once you have error message, update the error message in redis
		dbMappedData["ResponseMessage"] = "shouldHitVendor is off for email " + msg.Email
		if err := channelHelper.HandleShouldHitVendorOffError(msg.Mobile, msg.Channel, msg.Stage); err != nil {
			utils.Error(fmt.Errorf("failed to handle shouldHitVendor off error: %v", err))
		}
	}

	return true, dbMappedData, nil // message processed but not sent as response.IsSent is false
}
