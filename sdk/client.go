package sdk

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

type CommSdkClient struct {
	ClientName   string
	isAuthed     bool
	Channel      string
	AwsSnsClient *sns.SNS
}

func NewSdkClient(username, password, channel string) (*CommSdkClient, error) {
	snsClient, err := config.LoadSDKConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SDK Client: failed to load configs: %v", err)
	}

	var userName string
	var ok bool
	if ok, userName, channel = ValidateClient(username, password, channel); !ok {
		return nil, fmt.Errorf("client is not authenticated with us. Wrong Username or password")
	}

	return &CommSdkClient{
		ClientName:   userName,
		isAuthed:     ok,
		Channel:      channel,
		AwsSnsClient: snsClient,
	}, nil
}

func ValidateClient(username, password, channel string) (bool, string, string) {

	apiUrl := config.SdkConfigs.BasicAuthApiUrl

	apiHeaders := map[string]string{
		"Content-Type": "application/json",
		"Channel":      channel,
	}

	requestBody := map[string]interface{}{
		"username": username,
		"password": password,
	}

	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeaders, "", "", requestBody, variables.ContentTypeJSON)
	if err != nil {
		return false, "", ""
	}

	if apiResponse["ApistatusCode"].(int) == 200 {
		return true, apiResponse["user"].(string), apiResponse["channel"].(string)
	}
	return false, "", ""
}
