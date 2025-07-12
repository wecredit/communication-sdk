package email

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	sinchEmail "github.com/wecredit/communication-sdk/internal/channels/email/sinch"
	"github.com/wecredit/communication-sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendEmailByProcess(msg sdkModels.CommApiRequestBody) (bool, error) {

	requestBody := extapimodels.EmailRequestBody{
		Email:             msg.Email,
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
		return false, errors.New("template data not found in cache")
	}
	data, matchedVendor, err := channelHelper.FetchTemplateData(msg, templateDetails)
	if err != nil {
		channelHelper.LogTemplateNotFound(msg, err)
		if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, map[string]interface{}{
			"CommId":          msg.CommId,
			"Vendor":          msg.Vendor,
			"MobileNumber":    msg.Mobile,
			"IsSent":          false,
			"ResponseMessage": fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and Vendor: %s", msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor),
		}); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
			return false, nil
		}
		return false, nil
	}
	msg.Vendor = matchedVendor

	channelHelper.PopulateEmailFields(&requestBody, data)

	var response extapimodels.EmailResponse

	// Hit Into Email
	switch msg.Vendor {
	case variables.TIMES:
		return false, errors.New("times email is not supported yet")
	case variables.SINCH:
		response = sinchEmail.HitSinchEmailApi(requestBody)
	}
	
	response.TemplateName = requestBody.TemplateName
	response.CommId = msg.CommId
	response.Vendor = msg.Vendor
	response.Email = msg.Email

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}

	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("EmailResponse: %s", string(jsonBytes)))
	if response.IsSent {
		utils.Info(fmt.Sprintf("Email sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return true, nil
	}

	return false, nil
}
