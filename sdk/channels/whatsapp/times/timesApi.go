package timesWhatsapp

import (
	"fmt"
	"strings"

	timespayloads "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/times/timesPayloads"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/models/apiModels"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
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

	fmt.Println("Times Whatsapp payload:", timesApiModel)

	apiResponse, err := utils.ApiHit("POST", apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
	}

	response.StatusCode = apiResponse["ApistatusCode"].(int)
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
