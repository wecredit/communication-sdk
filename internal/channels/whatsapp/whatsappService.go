package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/config"
	sinchWhatsapp "github.com/wecredit/communication-sdk/internal/channels/whatsapp/sinch"
	timesWhatsapp "github.com/wecredit/communication-sdk/internal/channels/whatsapp/times"
	"github.com/wecredit/communication-sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/redis"
	services "github.com/wecredit/communication-sdk/internal/services/dbService"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendWpByProcess(msg sdkModels.CommApiRequestBody) (bool, error) {
	requestBody := extapimodels.WhatsappRequestBody{
		Mobile:            msg.Mobile,
		Process:           msg.ProcessName,
		Client:            msg.Client,
		EmiAmount:         msg.EmiAmount,
		CustomerName:      msg.CustomerName,
		LoanId:            msg.LoanId,
		ApplicationNumber: msg.ApplicationNumber,
		DueDate:           msg.DueDate,
	}

	utils.Debug("Fetching WHATSAPP process data from cache")
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
			utils.Error(fmt.Errorf("no template found for the given Process: %s, Stage: %.2f and Channel: %s and Active Lender", msg.ProcessName, msg.Stage, msg.Channel))
			if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, map[string]interface{}{
				"CommId":          msg.CommId,
				"Vendor":          msg.Vendor,
				"MobileNumber":    msg.Mobile,
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

	if templateName, exists := data["TemplateName"]; exists && templateName != nil {
		requestBody.TemplateName = templateName.(string)
	}
	if imageUrl, exists := data["ImageUrl"]; exists && imageUrl != nil {
		requestBody.ImageUrl = imageUrl.(string)
	}
	if imageId, exists := data["ImageId"]; exists && imageId != nil {
		requestBody.ImageID = imageId.(string)
	}
	if buttonLink, exists := data["Link"]; exists && buttonLink != nil {
		requestBody.ButtonLink = buttonLink.(string)
	}

	if templateVariables, exists := data["TemplateVariables"]; exists && templateVariables != nil {
		requestBody.TemplateVariables = templateVariables.(string)
	}

	if templateCategory, exists := data["TemplateCategory"]; exists && templateCategory != nil {
		requestBody.TemplateCategory = strconv.Itoa(int(templateCategory.(int64)))
	}

	var response extapimodels.WhatsappResponse

	// Hit Into WP
	switch msg.Vendor {
	case variables.TIMES:
		response = timesWhatsapp.HitTimesWhatsappApi(requestBody)
	case variables.SINCH:
		response = sinchWhatsapp.HitSinchWhatsappApi(requestBody)
	}

	response.CommId = msg.CommId
	response.TemplateName = requestBody.TemplateName
	response.Vendor = msg.Vendor
	response.MobileNumber = msg.Mobile

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	if err := database.InsertData(config.Configs.WhatsappOutputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}
	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("Whatsapp Response: %s", string(jsonBytes)))
	if response.IsSent {
		utils.Info(fmt.Sprintf("WhatsApp sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		if msg.Client == variables.CreditSea {
			redis.IncrementCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount)
		}
		return true, nil
	}
	return false, nil
}
