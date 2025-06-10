package sdk

import (
	"fmt"

	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/services"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func (c *CommSdkClient) Send(msg *sdkModels.CommApiRequestBody) (*sdkModels.CommApiResponseBody, error) {
	if c == nil {
		return &sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("please initialize the client first")
	}
	if !c.isAuthed {
		return &sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("unauthorized client")
	}
	if c.Channel != msg.Channel {
		return &sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("channel mismatch: expected %s, got %s", c.Channel, msg.Channel)
	}
	if c.AwsSnsClient == nil {
		return &sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("aws sns client not initialized")
	}

	msg.Client = c.ClientName

	response, err := sdkServices.ProcessCommApiData(msg, c.AwsSnsClient)
	if err != nil {
		utils.Error(fmt.Errorf("error in processing message: %v", err))
		return &sdkModels.CommApiResponseBody{Success: false}, err
	}

	return &response, nil
}
