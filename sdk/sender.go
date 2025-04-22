package sdk

import (
	"fmt"

	rcs "github.com/wecredit/communication-sdk/sdk/channels/rcs/sinch"
	"github.com/wecredit/communication-sdk/sdk/channels/whatsapp"
	"github.com/wecredit/communication-sdk/sdk/config"
	"github.com/wecredit/communication-sdk/sdk/internal/utils"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func Send(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	err := config.LoadConfigs()
	if err != nil {
		utils.Error(err)
	}
	utils.Debug("Configurations loaded successfully.")

	utils.Debug(fmt.Sprintf("Channel: %s", msg.Channel))

	switch msg.Channel {
	case variables.WhatsApp:
		return whatsapp.SendWpByProcess(msg)
	case variables.RCS:
		return rcs.SendRCSMessage(msg)
	default:
		return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("invalid channel: %s", msg.Channel)
	}
}
