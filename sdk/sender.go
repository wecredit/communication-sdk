package sdk

import (
	"fmt"

	rcs "dev.azure.com/wctec/communication-engine/sdk/channels/rcs/sinch"
	"dev.azure.com/wctec/communication-engine/sdk/channels/whatsapp"
	"dev.azure.com/wctec/communication-engine/sdk/config"
	"dev.azure.com/wctec/communication-engine/sdk/internal/models/sdkModels"
	"dev.azure.com/wctec/communication-engine/sdk/internal/utils"
	"dev.azure.com/wctec/communication-engine/sdk/variables"
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
