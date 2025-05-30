package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	sinchWhatsapp "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/sinch"
	timesWhatsapp "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/times"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendWpByProcess(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	requestBody := extapimodels.WhatsappRequestBody{
		Mobile:  msg.Mobile,
		Process: msg.ProcessName,
	}

	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return sdkModels.CommApiResponseBody{}, errors.New("template data not found in cache")
	}

	vendorDetails, found := cache.GetCache().GetMappedData(cache.VendorsData)
	if !found {
		utils.Error(fmt.Errorf("vendor data not found in cache"))
		return sdkModels.CommApiResponseBody{}, errors.New("vendor data not found in cache")
	}
	fmt.Println("Vendor Details:", vendorDetails)

	key := fmt.Sprintf("Process:%s|Stage:%s|Channel:%s|Vendor:%s", msg.ProcessName, strconv.Itoa(msg.Stage), msg.Channel, msg.Vendor)
	var data map[string]interface{}
	var ok bool
	var matchedVendor string
	if data, ok = templateDetails[key]; !ok {
		fmt.Println("No template found for the given key:", key)
		for otherKey, val := range templateDetails {
			if strings.HasPrefix(otherKey, fmt.Sprintf("Process:%s|Stage:%d|Channel:%s|Vendor:", msg.ProcessName, msg.Stage, msg.Channel)) {
				fmt.Printf("Found fallback template with key: %s\n", otherKey)
				data = val
				parts := strings.Split(otherKey, "|")
				if len(parts) == 4 {
					vendorPart := strings.TrimPrefix(parts[3], "Vendor:")
					matchedVendor = vendorPart
				}
				fmt.Println("matchedVendor:", matchedVendor)
				msg.Vendor = matchedVendor
				break
			}
		}
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
		return sdkModels.CommApiResponseBody{
			CommId:  msg.CommId,
			Success: true,
		}, nil
	}

	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to send message for source: %s", msg.Vendor)
}
