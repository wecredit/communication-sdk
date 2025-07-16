package sdk

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/sns"
	sdkConfig "github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

type CommSdkClient struct {
	ClientName   string
	isAuthed     bool
	Channel      string
	TopicArn     string
	AwsSnsClient *sns.SNS
}

func NewSdkClient(username, password, channel, baseUrl string) (*CommSdkClient, error) {
	snsClient, err := sdkConfig.LoadSDKConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SDK Client: failed to load configs: %v", err)
	}

	if username == "" || password == "" || channel == "" || baseUrl == "" {
		return nil, fmt.Errorf("username, password, channel, and baseUrl are required")
	}

	var userName, topicArn string
	var ok bool
	if ok, userName, channel, topicArn = ValidateClient(username, password, channel, baseUrl); !ok {
		return nil, fmt.Errorf("client is not authenticated with us for this channel. Wrong Username or password")
	}

	fmt.Println("TopicArn: ", topicArn)

	return &CommSdkClient{
		ClientName:   userName,
		isAuthed:     ok,
		Channel:      channel,
		TopicArn:     topicArn,
		AwsSnsClient: snsClient,
	}, nil
}

func ValidateClient(username, password, channel, baseUrl string) (bool, string, string, string) {

	apiUrl := baseUrl + "/clients/validate-client"

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
		return false, "", "", ""
	}

	if apiResponse["ApistatusCode"].(int) == 200 {
		return true, apiResponse["user"].(string), apiResponse["channel"].(string), apiResponse["topicArn"].(string)
	}
	return false, "", "", ""
}
