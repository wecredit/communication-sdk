package email

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/config"
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

	key := fmt.Sprintf("Process:%s|Stage:%.2f|Client:%s|Channel:%s|Vendor:%s", msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor)
	var data map[string]interface{}
	var ok, fallbackTemplatefound bool
	var matchedVendor string
	if data, ok = templateDetails[key]; !ok && msg.Client != variables.CreditSea {
		fmt.Println("No template found for the given key:", key)
		fallbackTemplatefound = false
		for otherKey, val := range templateDetails {
			if strings.HasPrefix(otherKey, fmt.Sprintf("Process:%s|Stage:%.2f|Client:%s|Channel:%s|Vendor:", msg.ProcessName, msg.Stage, msg.Client, msg.Channel)) {
				fmt.Printf("Found fallback template with key: %s\n", otherKey)
				fallbackTemplatefound = true
				data = val
				parts := strings.Split(otherKey, "|")
				if len(parts) == 5 {
					vendorPart := strings.TrimPrefix(parts[4], "Vendor:")
					matchedVendor = vendorPart
				}

				fmt.Println("Matched Vendor:", matchedVendor)
				vendorDetails, found := cache.GetCache().GetMappedData(cache.VendorsData)
				if !found {
					utils.Error(fmt.Errorf("vendor data not found in cache"))
				} else {
					key := fmt.Sprintf("Name:%s|Channel:%s", matchedVendor, msg.Channel)
					if vendorData, ok := vendorDetails[key]; ok {
						if vendorData["Status"].(int64) == variables.Inactive {
							utils.Error(fmt.Errorf("vendor %s is not active for channel %s", matchedVendor, msg.Channel))
							fallbackTemplatefound = false
						}
					}
				}

				msg.Vendor = matchedVendor

				break
			}
		}
		if !fallbackTemplatefound {
			utils.Error(fmt.Errorf("no template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and Active Lender", msg.ProcessName, msg.Stage, msg.Client, msg.Channel))
			if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, map[string]interface{}{
				"CommId":          msg.CommId,
				"Vendor":          msg.Vendor,
				"Email":           msg.Email,
				"IsSent":          false,
				"ResponseMessage": fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and active lender", msg.ProcessName, msg.Stage, msg.Client, msg.Channel),
			}); err != nil {
				utils.Error(fmt.Errorf("error inserting data into table: %v", err))
				return false, nil // TODO: Handle the case where insertion fails
			}
			return false, nil
		}
	} else if data, ok = templateDetails[key]; !ok && msg.Client == variables.CreditSea {
		utils.Error(fmt.Errorf("no template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and Vendor: %s", msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor))
		if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, map[string]interface{}{
			"CommId":          msg.CommId,
			"Vendor":          msg.Vendor,
			"Email":           msg.Email,
			"IsSent":          false,
			"ResponseMessage": fmt.Sprintf("No template found for the given Process: %s, Stage: %.2f, Client: %s, Channel: %s and Vendor: %s", msg.ProcessName, msg.Stage, msg.Client, msg.Channel, msg.Vendor),
		}); err != nil {
			utils.Error(fmt.Errorf("error inserting data into table: %v", err))
			return false, nil
		}
		return false, nil
	}

	if templateVariables, exists := data["TemplateVariables"]; exists && templateVariables != nil {
		requestBody.TemplateVariables = templateVariables.(string)
	}

	if templateName, exists := data["TemplateName"]; exists && templateName != nil {
		requestBody.TemplateName = templateName.(string)
	}

	if templateText, exists := data["TemplateText"]; exists && templateText != nil {
		requestBody.TemplateText = templateText.(string)
	}

	if templateCategory, exists := data["TemplateCategory"]; exists && templateCategory != nil {
		requestBody.TemplateCategory = strconv.Itoa(int(templateCategory.(int64)))
	}

	var response extapimodels.EmailResponse
	// Hit Into WP
	switch msg.Vendor {
	case variables.TIMES:
		// response = timesSms.HitTimesSmsApi(requestBody)
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
