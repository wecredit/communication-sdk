package sinchSms

import (
	"fmt"

	sinchpayloads "github.com/wecredit/communication-sdk/sdk/channels/sms/sinch/sinchPayloads"
	"github.com/wecredit/communication-sdk/sdk/config"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchSmsApi(data extapimodels.SmsRequestBody) extapimodels.SmsResponse {
	var sinchSmsResponse extapimodels.SmsResponse
	sinchSmsResponse.IsSent = false

	// Getting the API URL
	apiUrl := config.Configs.SinchSmsApiUrl

	// Setting the API header
	apiHeader := map[string]string{
		"Content-Type": "application/json",
	}

	// Get api payload
	apiPayload, err := sinchpayloads.GetTemplatePayload(data, config.Configs)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while getting SMS payload: %v", err))
		sinchSmsResponse.ResponseMessage = fmt.Sprintf("error occured while getting Sinch SMS payload: %v", err)
		return sinchSmsResponse
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeader, "", "", apiPayload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch SMS API: %v", err))
		sinchSmsResponse.ResponseMessage = fmt.Sprintf("error occured while hitting Sinch SMS payload: %v", err)
	}

	accepted := apiResponse["accepted"].(bool)

	if accepted {
		sinchSmsResponse.TransactionId = apiResponse["respid"].(string)
		sinchSmsResponse.IsSent = true
		sinchSmsResponse.ResponseMessage = "Message Submitted Successfully"
	} else {
		sinchSmsResponse.ResponseMessage = GetRejectionReason(apiResponse["error"].(string))
	}

	// TODO Handling For Api Responses

	// if code, ok := apiResponse["code"].(float64); ok {
	// 	response.StatusCode = code
	// } else {
	// 	return response, fmt.Errorf("unexpected type for code: %T", apiResponse["code"])
	// }
	// response.Message = apiResponse["message"].(string)
	// response.Status = apiResponse["status"].(bool)
	fmt.Println("Sinch SMS Final response:", sinchSmsResponse)

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
