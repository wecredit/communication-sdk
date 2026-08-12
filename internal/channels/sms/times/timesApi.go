package timesSms

import (
	"context"
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	timespayloads "github.com/wecredit/communication-sdk/internal/channels/sms/times/timesPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitTimesSmsApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var timesSmsResponse extapimodels.SmsResponse
	timesSmsResponse.IsSent = false
	timesSmsResponse.Outcome = outcome.FailedFinal

	apiUrl := config.Configs.TimesSmsApiUrl
	apiHeader := map[string]string{
		"Content-Type": "application/json",
	}

	apiPayload, err := timespayloads.GetTemplatePayload(data, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting SMS payload: %v", err))
		timesSmsResponse.ResponseMessage = fmt.Sprintf("Error in getting Times SMS Payload: %v", err)
		timesSmsResponse.Outcome = outcome.FailedFinal
		return timesSmsResponse
	}

	if err := ratelimit.WaitFor(context.Background(), ratelimit.Key(variables.TIMES, data.Client)); err != nil {
		timesSmsResponse.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		timesSmsResponse.Outcome = outcome.FailedRetryable
		return timesSmsResponse
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, config.Configs.TimesSmsApiUserName, config.Configs.TimesSmsApiPassword, apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Times Sms API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, data, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		timesSmsResponse.ResponseMessage = fmt.Sprintf("Error in hitting Times SMS API: %v", err)
		timesSmsResponse.Outcome = outcome.ClassifyTransportError(err)
		return timesSmsResponse
	}

	status, _ := apiResponse["state"].(string)
	description, _ := apiResponse["description"].(string)
	timesSmsResponse.ResponseMessage = fmt.Sprintf("%s:%s", status, description)
	if txn, ok := apiResponse["transactionId"].(float64); ok {
		timesSmsResponse.TransactionId = fmt.Sprintf("%d", int(txn))
	}

	if status == "SUBMIT_ACCEPTED" {
		timesSmsResponse.IsSent = true
		timesSmsResponse.Outcome = outcome.Submitted
		return timesSmsResponse
	}

	timesSmsResponse.Outcome = outcome.FailedFinal
	return timesSmsResponse
}
