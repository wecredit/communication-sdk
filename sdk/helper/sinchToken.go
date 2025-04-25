package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/utils"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

const (
	authURL  = "https://auth.aclwhatsapp.com/realms/ipmessaging/protocol/openid-connect/token"
	clientID = "ipmessaging-client"
	username = "YOUR_USERNAME"
	password = "YOUR_PASSWORD"
)

func GetNewToken() (*extapimodels.SinchTokenResponse, error) {
	// data := url.Values{}
	// data.Set("grant_type", "password")
	// data.Set("client_id", config.Configs.SinchClientId)
	// data.Set("username", config.Configs.SinchUserName)
	// data.Set("password", config.Configs.SinchPassword)

	tokenRequestBody := map[string]string{
		"grant_type": "password",
		"client_id":  config.Configs.SinchClientId,
		"username":   config.Configs.SinchUserName,
		"password":   config.Configs.SinchPassword,
	}

	tokenUrl := config.Configs.SinchTokenApiUrl

	apiHeaders := map[string]string{
		"Cache-Control": "no-cache",
		"Content-Type":  "application/x-www-form-urlencoded",
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, tokenUrl, apiHeaders, "", "", tokenRequestBody, variables.ContentTypeFormEncoded)
	if err != nil {
		utils.Error(fmt.Errorf("error occured while hitting into Sinch Generate Token API: %v", err))
	}

	fmt.Println("apiResponse:", apiResponse)

	var token extapimodels.SinchTokenResponse

	if apiResponse["ApistatusCode"].(int) == 200 {
		token.AccessToken = apiResponse["access_token"].(string)
		token.ExpiresIn = int(apiResponse["expires_in"].(float64))
		token.RefreshToken = apiResponse["refresh_token"].(string)
		token.RefreshExpiresIn = int(apiResponse["refresh_expires_in"].(float64))
		token.TokenType = apiResponse["token_type"].(string)
	}
	return &token, nil
}

func RefreshAccessToken(refreshToken string) (*extapimodels.SinchTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", config.Configs.SinchClientId)
	data.Set("refresh_token", refreshToken)

	req, _ := http.NewRequest("POST", authURL, bytes.NewBufferString(data.Encode()))
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var token extapimodels.SinchTokenResponse
	json.Unmarshal(body, &token)
	return &token, nil
}
