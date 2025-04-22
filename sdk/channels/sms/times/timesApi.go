package sms

import (
	"encoding/base64"
	"fmt"

	timespayloads "github.com/wecredit/communication-sdk/sdk/channels/sms/times/timesPayloads"
	"github.com/wecredit/communication-sdk/sdk/internal/models"
	apimodels "github.com/wecredit/communication-sdk/sdk/internal/models/apiModels"
	extapimodels "github.com/wecredit/communication-sdk/sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/internal/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitTimesApi(timesApiModel extapimodels.TimesAPIModel, config models.Config) apimodels.WpApiResponseData {
	var response apimodels.WpApiResponseData

	// Getting the API URL
	apiUrl := config.TimesSmsApiUrl

	// Getting the WhatsApp Authorization token
	// apiAuthorization := config.TimesApiToken

	username := config.TimesSmsApiUserName
	password := config.TimesSmsApiPassword
	credentials := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))

	// Setting the API header
	apiHeader := map[string]string{
		"Authorization": fmt.Sprintf("Basic %s", credentials),
		"Content-Type":  "application/json",
	}

	// Get api payload
	apiPayload, err := timespayloads.GetTemplatePayload(config)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting WP payload: %v", err))
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Wp API: %v", err))
	}

	// TODO Handling For Api Responses

	response.StatusCode = apiResponse["code"].(float64)
	response.Message = apiResponse["message"].(string)
	response.Status = apiResponse["status"].(bool)

	return response
}
