package sdk

import (
	"fmt"

	services "github.com/wecredit/communication-sdk/sdk/internal/services/apiServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

/* func SendWithoutClient(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	err := config.LoadConfigs()
	if err != nil {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error in loading configuration:%v", err)
	}
	utils.Debug("Configurations loaded successfully.")

	utils.Debug(fmt.Sprintf("Channel: %s", msg.Channel))

	// dbAnalytics, err = gorm.Open(sqlserver.Open(msg.DsnAnalytics), &gorm.Config{})
	// if err != nil {
	// 	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to connect to Analytical DB: %w", err)
	// }

	response, err := services.ProcessCommApiData(&msg)
	if err != nil {
		utils.Error(fmt.Errorf("error in processing message: %v", err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error in processing message: %v", err)
	}

	responseJSON, _ := json.Marshal(response)
	utils.Info("Response Data: " + string(responseJSON))

	return response, nil
} */

func (c *CommSdkClient) Send(msg *sdkModels.CommApiRequestBody) (*sdkModels.CommApiResponseBody, error) {
	if c == nil {
		return &sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("please initialize the client first")
	}
	if !c.isAuthed {
		return &sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("unauthorized client")
	}
	if c.QueueClient == nil {
		return &sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("queue client not initialized")
	}

	msg.Client = c.ClientName

	response, err := services.ProcessCommApiData(msg, c.QueueClient)
	if err != nil {
		utils.Error(fmt.Errorf("error in processing message: %v", err))
		return &sdkModels.CommApiResponseBody{Success: false}, err
	}

	return &response, nil
}
