package timesSms

import (
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	timespayloads "github.com/wecredit/communication-sdk/internal/channels/sms/times/timesPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitTimesSmsApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var timesSmsResponse extapimodels.SmsResponse
	timesSmsResponse.IsSent = false

	// Getting the API URL
	apiUrl := config.Configs.TimesSmsApiUrl

	// Getting the WhatsApp Authorization token
	// apiAuthorization := config.Configs.TimesApiToken

	// username := config.Configs.TimesSmsApiUserName
	// password := config.Configs.TimesSmsApiPassword
	// credentials := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))

	// Setting the API header
	apiHeader := map[string]string{
		"Content-Type": "application/json",
	}

	// Get api payload
	apiPayload, err := timespayloads.GetTemplatePayload(data, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting SMS payload: %v", err))
		timesSmsResponse.ResponseMessage = fmt.Sprintf("Error in getting Times SMS Payload: %v", err)
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, config.Configs.TimesSmsApiUserName, config.Configs.TimesSmsApiPassword, apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Sms API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, data, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		timesSmsResponse.ResponseMessage = fmt.Sprintf("Error in hitting Times SMS API: %v", err)
		return timesSmsResponse
	}

	status := apiResponse["state"].(string)
	description := apiResponse["description"].(string)

	timesSmsResponse.ResponseMessage = fmt.Sprintf("%s:%s", status, description)
	timesSmsResponse.TransactionId = fmt.Sprintf("%d", int(apiResponse["transactionId"].(float64)))

	if status == "SUBMIT_ACCEPTED" {
		timesSmsResponse.IsSent = true
	}

	fmt.Println("TimesSMSResponseFinal:", timesSmsResponse)

	return timesSmsResponse
}
