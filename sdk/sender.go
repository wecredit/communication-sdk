package sdk

import (
	"encoding/json"
	"fmt"

	"github.com/wecredit/communication-sdk/sdk/config"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/apiServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

var (
	dbAnalytics *gorm.DB
)

func Send(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	err := config.LoadConfigs()
	if err != nil {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error in loading configuration:%v", err)
	}
	utils.Debug("Configurations loaded successfully.")

	utils.Debug(fmt.Sprintf("Channel: %s", msg.Channel))

	dbAnalytics, err = gorm.Open(sqlserver.Open(msg.DsnAnalytics), &gorm.Config{})
	if err != nil {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to connect to Analytical DB: %w", err)
	}

	response, err := services.ProcessCommApiData(msg)
	if err != nil {
		utils.Error(fmt.Errorf("error in processing message: %v", err))
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("error in processing message: %v", err)
	}

	responseJSON, _ := json.Marshal(response)
	utils.Info("Response Data: " + string(responseJSON))

	return response, nil

	// switch msg.Channel {
	// case variables.WhatsApp:
	// 	return whatsapp.SendWpByProcess(dbAnalytics, msg)
	// case variables.RCS:
	// 	return rcs.SendRCSMessage(msg)
	// default:
	// 	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("invalid channel: %s", msg.Channel)
	// }
}
