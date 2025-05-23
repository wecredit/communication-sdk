package sdk

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

type CommSdkClient struct {
	ClientName  string
	isAuthed    bool
	QueueClient *azservicebus.Client
}

func NewSdkClient(username, password string) (*CommSdkClient, error) {
	queueClient, err := config.LoadSDKConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to load configs: %v", err)
	}
	var userName string
	var ok bool
	if ok, userName = ValidateClient(username, password); !ok {
		return nil, fmt.Errorf("client is not authenticated with us. Wrong Username or password")
	} else {
		return &CommSdkClient{
			ClientName:  userName,
			isAuthed:    ok,
			QueueClient: queueClient,
		}, nil
	}
}

func ValidateClient(username, password string) (bool, string) {

	apiUrl := config.SdkConfigs.BasicAuthApiUrl

	apiHeaders := map[string]string{
		"Content-Type": "application/json",
	}

	requestBody := map[string]interface{}{
		"username": username,
		"password": password,
	}
	
	apiResponse, err := utils.ApiHit(variables.PostMethod, apiUrl, apiHeaders, "", "", requestBody, variables.ContentTypeJSON)
	if err != nil {
		return false, ""
	}

	if apiResponse["ApistatusCode"].(int) == 200 {
		return true, apiResponse["user"].(map[string]interface{})["username"].(string)
	}
	return false, ""
}
