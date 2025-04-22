package sdk

import (
	"fmt"

	rcs "github.com/wecredit/communication-sdk/sdk/channels/rcs/sinch"
	"github.com/wecredit/communication-sdk/sdk/channels/whatsapp"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/utils"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
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

	dbAnalytics, err = gorm.Open(sqlserver.Open(msg.DSN), &gorm.Config{})
	if err != nil {
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to connect to Analytical DB: %w", err)
	}

	switch msg.Channel {
	case variables.WhatsApp:
		return whatsapp.SendWpByProcess(dbAnalytics, msg)
	case variables.RCS:
		return rcs.SendRCSMessage(msg)
	default:
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("invalid channel: %s", msg.Channel)
	}
}
