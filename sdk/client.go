package sdk

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/wecredit/communication-sdk/sdk/config"
)

type CommSdkClient struct {
	Username    string
	Password    string
	isAuthed    bool
	QueueClient *azservicebus.Client
}

func NewSdkClient(username, password string) (*CommSdkClient, error) {
	err := config.LoadConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to load configs: %v", err)
	}

	/*
		// Validate user credentials
		if !validateUser(username, password) {
			return nil, fmt.Errorf("invalid credentials")
		}
	*/
	return &CommSdkClient{
		Username: username,
		Password: password,
		isAuthed: true,
	}, nil
}
