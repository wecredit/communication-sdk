package sinchRcs

import (
	"fmt"
	"strings"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func HitSinchRcsApi(data extapimodels.RcsRequesBody) extapimodels.RcsResponse {
	var responseBody extapimodels.RcsResponse
	responseBody.IsSent = false
	rcsApiUrl := config.Configs.SinchRcsApiUrl
	rcsApiUrl = fmt.Sprintf("%s%s%s", config.Configs.SinchRcsApiUrl, data.ProjectId, "/messages:send")
	accessToken, ok := cache.GetAccessToken()
	if !ok {
		token, err := helper.GetNewToken()
		if err != nil {
			responseBody.ResponseMessage = fmt.Sprintf("%v", err)
			return responseBody
		}
		cache.SetToken(token)
		accessToken = token.AccessToken
	}

	apiHeaders := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
	}

	payload := extapimodels.SinchRcsPayload{
		AppID: data.AppIdKey,
	}
	payload.Recipient.IdentifiedBy.ChannelIdentities = []struct {
		Channel  string `json:"channel"`
		Identity string `json:"identity"`
	}{
		{Channel: "RCS", Identity: fmt.Sprintf("91%s", data.Mobile)},
	}
	payload.Message.TemplateMessage.ChannelTemplate.RCS.TemplateId = data.TemplateName
	payload.Message.TemplateMessage.ChannelTemplate.RCS.LanguageCode = "en"

	apiResponse, err := utils.ApiHit(variables.PostMethod, rcsApiUrl, apiHeaders, "", "", payload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch RCS API: %v", err))
	}

	if apiResponse["ApistatusCode"].(int) == 200 {
		utils.Info("RCS message sent successfully")
		responseBody.IsSent = true
		responseBody.TransactionId = apiResponse["message_id"].(string)
		responseBody.ResponseMessage = "Message Submitted Successfully"
		return responseBody
	}

	if apiResponse["ApistatusCode"].(int) != 200 {
		errMap, ok := apiResponse["error"].(map[string]interface{})
		if !ok {
			utils.Error(fmt.Errorf("unexpected error format: %v", apiResponse["error"]))
			responseBody.ResponseMessage = "Unknown error format"
			return responseBody
		}

		var errorMsgs []string

		// Step 1: Add the top-level message if present
		if msg, ok := errMap["message"].(string); ok && msg != "" {
			errorMsgs = append(errorMsgs, msg)
		}

		// Step 2: Dynamically parse all details, regardless of type
		if details, ok := errMap["details"].([]interface{}); ok {
			for _, d := range details {
				detail, ok := d.(map[string]interface{})
				if !ok {
					continue
				}

				// Extract generic description if available
				if desc, ok := detail["description"].(string); ok && desc != "" {
					errorMsgs = append(errorMsgs, desc)
				}

				// Extract ResourceInfo info
				if resType, ok := detail["resource_type"].(string); ok {
					resourceName := detail["resource_name"]
					errorMsgs = append(errorMsgs, fmt.Sprintf("Missing resource: %v (%v)", resourceName, resType))
				}

				// Extract BadRequest field_violations
				if violations, ok := detail["field_violations"].([]interface{}); ok {
					for _, v := range violations {
						if violation, ok := v.(map[string]interface{}); ok {
							field := violation["field"]
							desc := violation["description"]
							errorMsgs = append(errorMsgs, fmt.Sprintf("%v: %v", field, desc))
						}
					}
				}

				// Catch any other unexpected structures
				for k, v := range detail {
					if k != "@type" && k != "field_violations" && k != "description" && k != "resource_type" && k != "resource_name" {
						errorMsgs = append(errorMsgs, fmt.Sprintf("%v: %v", k, v))
					}
				}
			}
		}

		// Step 3: Fallback if still empty
		finalErrMsg := strings.Join(errorMsgs, " | ")
		if finalErrMsg == "" {
			finalErrMsg = fmt.Sprintf("Error Code: %v, Status: %v", errMap["code"], errMap["status"])
		}

		utils.Error(fmt.Errorf("response failed with status: %v", finalErrMsg))
		responseBody.ResponseMessage = finalErrMsg
		return responseBody
	}

	return responseBody
}
