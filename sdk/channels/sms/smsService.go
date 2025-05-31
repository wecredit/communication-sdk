package sms

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	sinchSms "github.com/wecredit/communication-sdk/sdk/channels/sms/sinch"
	timesSms "github.com/wecredit/communication-sdk/sdk/channels/sms/times"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/dbService"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendSmsByProcess(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {

	requestBody := extapimodels.SmsRequestBody{
		Mobile:  msg.Mobile,
		Process: msg.ProcessName,
	}

	utils.Debug("Fetching SMS process data from cache")
	templateDetails, found := cache.GetCache().GetMappedData(cache.TemplateDetailsData)
	if !found {
		utils.Error(fmt.Errorf("template data not found in cache"))
		return sdkModels.CommApiResponseBody{}, errors.New("template data not found in cache")
	}

	key := fmt.Sprintf("Process:%s|Stage:%s|Channel:%s|Vendor:%s", msg.ProcessName, strconv.Itoa(msg.Stage), msg.Channel, msg.Vendor)
	var data map[string]interface{}
	var ok, fallbackTemplatefound bool
	var matchedVendor string
	if data, ok = templateDetails[key]; !ok {
		fmt.Println("No template found for the given key:", key)
		fallbackTemplatefound = false
		for otherKey, val := range templateDetails {
			if strings.HasPrefix(otherKey, fmt.Sprintf("Process:%s|Stage:%d|Channel:%s|Vendor:", msg.ProcessName, msg.Stage, msg.Channel)) {
				fmt.Printf("Found fallback template with key: %s\n", otherKey)
				fallbackTemplatefound = true
				data = val
				parts := strings.Split(otherKey, "|")
				if len(parts) == 4 {
					vendorPart := strings.TrimPrefix(parts[3], "Vendor:")
					matchedVendor = vendorPart
				}
				msg.Vendor = matchedVendor
				break
			}
		}
		if !fallbackTemplatefound {
			utils.Error(fmt.Errorf("no template found for the given Process: %s, Stage: %s and Channel: %s", msg.ProcessName, strconv.Itoa(msg.Stage), msg.Channel))
			return sdkModels.CommApiResponseBody{}, fmt.Errorf("no template found for the given Process: %s, Stage: %s and Channel: %s", msg.ProcessName, strconv.Itoa(msg.Stage), msg.Channel)
		}
	}

	if dltTemplateId, exists := data["DltTemplateId"]; exists && dltTemplateId != nil {
		requestBody.DltTemplateId = dltTemplateId.(int64)
	}
	if templateText, exists := data["TemplateText"]; exists && templateText != nil {
		requestBody.TemplateText = templateText.(string)
	}

	var response extapimodels.SmsResponse
	// Hit Into WP
	switch msg.Vendor {
	case variables.TIMES:
		response = timesSms.HitTimesSmsApi(requestBody)
	case variables.SINCH:
		response = sinchSms.HitSinchSmsApi(requestBody)
	}
	response.DltTemplateId = requestBody.DltTemplateId
	response.CommId = msg.CommId
	response.Vendor = msg.Vendor

	dbMappedData, err := services.MapIntoDbModel(response)
	if err != nil {
		utils.Error(fmt.Errorf("error in mapping data into dbModel: %v", err))
	}

	if err := database.InsertData(config.Configs.SmsOutputTable, database.DBtech, dbMappedData); err != nil {
		utils.Error(fmt.Errorf("error inserting data into table: %v", err))
	}

	jsonBytes, _ := json.Marshal(response)
	utils.Debug(fmt.Sprintf("SmsResponse: %s", string(jsonBytes)))
	if response.IsSent {
		utils.Info(fmt.Sprintf("SMS sent successfully for Process: %s on %s through %s", msg.ProcessName, msg.Mobile, msg.Vendor))
		return sdkModels.CommApiResponseBody{
			CommId:  msg.CommId,
			Success: true,
		}, nil
	}
	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to send message for process: %s", msg.ProcessName)
}
