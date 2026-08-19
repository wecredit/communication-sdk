package sinchSms

import (
	"context"
	"fmt"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	sinchpayloads "github.com/wecredit/communication-sdk/internal/channels/sms/sinch/sinchPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/ratelimit"
	"github.com/wecredit/communication-sdk/sdk/queue"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchSmsApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var sinchSmsResponse extapimodels.SmsResponse
	sinchSmsResponse.IsSent = false
	sinchSmsResponse.Outcome = outcome.FailedFinal

	apiUrl := config.Configs.SinchSmsApiUrl
	apiHeader := map[string]string{
		"Content-Type": "application/json",
	}

	apiPayload, err := sinchpayloads.GetTemplatePayload(data, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting SMS payload: %v", err))
		sinchSmsResponse.ResponseMessage = fmt.Sprintf("error occured in Sinch SMS payload: %v for %s", err, data.Client)
		sinchSmsResponse.Outcome = outcome.FailedFinal
		return sinchSmsResponse
	}

	if err := ratelimit.WaitFor(context.Background(), ratelimit.Key(variables.SINCH, data.Client)); err != nil {
		sinchSmsResponse.ResponseMessage = fmt.Sprintf("rate limit wait cancelled: %v", err)
		sinchSmsResponse.Outcome = outcome.FailedRetryable
		return sinchSmsResponse
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch SMS API: %v", err))
		if queueErr := queue.SendMessageWithSubject(queue.SQSClient, data, config.Configs.AwsErrorQueueUrl, variables.ApiHitsFails, err.Error()); queueErr != nil {
			utils.Error(fmt.Errorf("error sending message to error queue: %v", queueErr))
		}
		sinchSmsResponse.ResponseMessage = fmt.Sprintf("error occured while hitting Sinch SMS payload: %v", err)
		sinchSmsResponse.Outcome = outcome.ClassifyTransportError(err)
		return sinchSmsResponse
	}

	accepted, _ := apiResponse["accepted"].(bool)
	if accepted {
		if respid, ok := apiResponse["respid"].(string); ok {
			sinchSmsResponse.TransactionId = respid
		}
		sinchSmsResponse.IsSent = true
		sinchSmsResponse.Outcome = outcome.Submitted
		sinchSmsResponse.ResponseMessage = "Message Submitted Successfully"
		return sinchSmsResponse
	}

	errCode, _ := apiResponse["error"].(string)
	sinchSmsResponse.ResponseMessage = GetRejectionReason(errCode)
	sinchSmsResponse.Outcome = outcome.FailedFinal
	return sinchSmsResponse
}

// RejectionCodeMap maps rejection codes to their descriptions
var RejectionCodeMap = map[string]string{
	"-1":  "User Id/ Password Incorrect or Appid Missing",
	"-2":  "User Id Missing",
	"-3":  "Password Missing",
	"-4":  "Content type Missing",
	"-5":  "Sender Missing",
	"-6":  "MSISDN Missing",
	"-7":  "Message Text Missing",
	"-8":  "Message Id Missing",
	"-9":  "WAP Push URL Missing",
	"-10": "Authentication Failed",
	"-11": "Service Blocked for User",
	"-12": "Repeated Message Id Received",
	"-13": "Invalid Content Type Received",
	"-14": "International Messages Not Allowed",
	"-15": "Incomplete or Invalid XML Packet Received",
	"-16": "Invalid alert Flag value",
	"-17": "Direct Pushing Not Allowed",
	"-18": "CLI not registered",
	"-19": "Operator Specific MSISDN Blocked",
	"-27": "Block Text (entire string or single word) & MSISDN",
	"-41": "ACL_ERROR_INVALID_SHORTEN_FLAG",
	"-42": "ACL_ERROR_SHORTENING_NOT_ALLOWED",
	"-43": "ACL_ERROR_INVALID_DOMAIN",
	"-44": "ACL_ERROR_INVALID_ALIAS",
	"-45": "ACL_ERROR_INVALID_FORWARD",
	"-46": "ACL_ERROR_FORWARD_NOT_ALLOWED",
	"-47": "ACL_ERROR_INVALID_DYNAMIC",
	"-48": "ACL_ERROR_DYNAMIC_REDIRECTION_NOT_ALLOWED",
	"-49": "ACL_ERROR_FALLBACK_DESTINATION_NOT_DEFINED",
	"-50": "ACL_ERROR_INVALID_DESTINATION",
	"-51": "ACL_ERROR_MISSING_DESTINATION",
	"-75": "ACL_ERROR_INVALID_JSONEXCEPTION",
	"-76": "ACL_ERROR_INVALID_ENCRYPTED_DATA",
	"-77": "ACL_ERROR_ACCESSTOKEN_NOT_FOUND",
	"-78": "ACL_ERROR_ACCESSTOKEN_EXPIRED",
	"-79": "JSON batch size exceeded",
}

// GetRejectionReason returns the mapped description for a given rejection code
func GetRejectionReason(code string) string {
	if reason, exists := RejectionCodeMap[code]; exists {
		return fmt.Sprintf("Code %s: %s", code, reason)
	}
	return fmt.Sprintf("Code %s: Unknown rejection reason", code)
}
