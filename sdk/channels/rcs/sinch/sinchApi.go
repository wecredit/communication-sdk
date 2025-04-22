package rcs

import (
	"fmt"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/internal/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/internal/utils"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendRCSMessage(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {

	rcsApiUrl := config.Configs.SinchRcsApiUrl

	accessToken, ok := cache.GetAccessToken()
	if !ok {
		token, err := helper.GetNewToken()
		if err != nil {
			return sdkModels.CommApiResponseBody{Success: false}, err
		}
		cache.SetToken(token)
		accessToken = token.AccessToken
	}

	apiHeaders := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
	}

	payload := extapimodels.SinchRcsPayload{
		AppID: "01JR7CR52EDC4YZV10B73A9FXX",
	}
	payload.Recipient.IdentifiedBy.ChannelIdentities = []struct {
		Channel  string `json:"channel"`
		Identity string `json:"identity"`
	}{
		{Channel: "RCS", Identity: "919123359755"},
	}
	payload.Message.CardMessage.Title = "Sign up for our newsletter"
	payload.Message.CardMessage.Description = "Our weekly newsletter with deals and extra content!"
	payload.Message.CardMessage.Media.URL = "https://sample-videos.com/img/Sample-jpg-image-100kb.jpg"
	payload.Message.CardMessage.Height = "MEDIUM"
	payload.Message.CardMessage.Choices = []struct {
		UrlMessage struct {
			Url string `json:"url"`
		} `json:"url_message"`
		PostbackData string `json:"postback_data"`
	}{
		{UrlMessage: struct {
			Url string `json:"url"`
		}{Url: "Sign up here"}, PostbackData: "Hello"},
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, rcsApiUrl, apiHeaders, "", "", payload, variables.ContentTypeJSON)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch RCS API: %v", err))
	}

	fmt.Println("apiresponse for sinch rcs", apiResponse)

	if apiResponse["ApistatusCode"].(int) != 200 {
		err := apiResponse["error"].(map[string]interface{})
		utils.Error(fmt.Errorf("response failed with status: %v", err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("response failed with status: %v", err)
	}

	if apiResponse["ApistatusCode"].(int) == 200 {
		utils.Info("RCS message sent successfully")
		return sdkModels.CommApiResponseBody{Success: true}, nil
	}
	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("response failed with status: %v", apiResponse["error"])
}
