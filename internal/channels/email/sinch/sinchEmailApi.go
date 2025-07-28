package sinchEmail

import (
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	sinchpayloads "github.com/wecredit/communication-sdk/internal/channels/email/sinch/sinchPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchEmailApi(data extapimodels.EmailRequestBody) extapimodels.EmailResponse {
	var sinchEmailResponse extapimodels.EmailResponse
	sinchEmailResponse.IsSent = false

	// Getting the API URL
	apiUrl := config.Configs.SinchEmailApiUrl

	// Setting the API header
	apiHeader := map[string]string{
		"Cache-Control": "no-cache",
		"Authorization": fmt.Sprintf("Bearer %s", config.Configs.SinchEmailApiToken),
		"Content-Type":  "application/json",
	}

	// Get api payload
	apiPayload, err := sinchpayloads.GetTemplatePayload(data, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting Email payload: %v", err))
		sinchEmailResponse.ResponseMessage = fmt.Sprintf("error occured in Sinch Email payload: %v for %s", err, data.Client)
		return sinchEmailResponse
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Email API: %v", err))
		sinchEmailResponse.ResponseMessage = fmt.Sprintf("error occured while hitting Sinch Email payload: %v", err)
		return sinchEmailResponse
	}

	status, ok := apiResponse["ApistatusCode"].(int)
	if !ok {
		sinchEmailResponse.ResponseMessage = fmt.Sprintf("error occured while hitting Sinch Email payload: %v", err)
		return sinchEmailResponse
	}

	accepted := status == 200

	if accepted {
		sinchEmailResponse.TransactionId = apiResponse["request_id"].(string)
		sinchEmailResponse.IsSent = true
		sinchEmailResponse.ResponseMessage = "Message Submitted Successfully"
	} else {
		sinchEmailResponse.ResponseMessage = apiResponse["errors"].(string)
	}

	fmt.Println("Sinch Email Final response:", sinchEmailResponse)

	return sinchEmailResponse
}
