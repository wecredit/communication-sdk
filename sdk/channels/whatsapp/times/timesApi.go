package timesWhatsapp

import (
	"fmt"
	"strings"

	timespayloads "dev.azure.com/wctec/communication-engine/sdk/channels/whatsapp/times/timesPayloads"
	"dev.azure.com/wctec/communication-engine/sdk/config"
	"dev.azure.com/wctec/communication-engine/sdk/internal/models/apiModels"
	extapimodels "dev.azure.com/wctec/communication-engine/sdk/internal/models/extApiModels"
	"dev.azure.com/wctec/communication-engine/sdk/internal/utils"
	"dev.azure.com/wctec/communication-engine/sdk/variables"
)

func HitTimesWhatsappApi(timesApiModel extapimodels.TimesAPIModel) apiModels.WpApiResponseData {
	var response apiModels.WpApiResponseData

	// Getting the API URL
	apiUrl := config.Configs.TimesWpApiUrl

	// Getting the WhatsApp Authorization token
	apiAuthorization := config.Configs.TimesWpAPIToken

	// Setting the API header
	apiHeader := map[string]string{
		"Authorization": apiAuthorization,
		"Content-Type":  "application/json",
	}

	// Get api payload
	apiPayload, err := getPayload(timesApiModel)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	apiResponse, err := utils.ApiHit("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
	}

	response.StatusCode = apiResponse["code"].(float64)
	response.Message = apiResponse["message"].(string)
	response.Status = apiResponse["status"].(bool)

	return response
}

func getPayload(timesApiModel extapimodels.TimesAPIModel) (map[string]interface{}, error) {
	if strings.Contains(timesApiModel.Process, "utility") {
		// For Utility Payload
		return timespayloads.GetTimesUtilityPayload(timesApiModel)
	} else {
		return timespayloads.GetTimesMediaPayload(timesApiModel)
	}

}
