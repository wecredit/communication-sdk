package whatsapp

import (
	"fmt"

	sinchWhatsapp "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/sinch"
	timesWhatsapp "github.com/wecredit/communication-sdk/sdk/channels/whatsapp/times"
	"github.com/wecredit/communication-sdk/sdk/internal/database"
	extapimodels "github.com/wecredit/communication-sdk/sdk/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func SendWpByProcess(msg sdkModels.CommApiRequestBody) (sdkModels.CommApiResponseBody, error) {
	var timeData extapimodels.TimesAPIModel
	var sinchData extapimodels.SinchAPIModel

	timeData.Mobile = msg.Mobile
	timeData.Process = msg.ProcessName

	sinchData.Mobile = msg.Mobile
	sinchData.Process = msg.ProcessName

	utils.Debug("Fetching whatsapp process data")
	wpProcessData, err := database.GetWhatsappProcessData(database.DBanalytics, msg.ProcessName, msg.Vendor)
	if err != nil {
		utils.Error(fmt.Errorf("error occurred while fetching WhatsApp process data for process '%s': %v", msg.ProcessName, err))
		return sdkModels.CommApiResponseBody{}, fmt.Errorf("error occurred while fetching WhatsApp process data for process '%s': %v", msg.ProcessName, err)
	}

	for _, record := range wpProcessData {
		if templateName, exists := record["template_name"]; exists && templateName != nil {
			timeData.TemplateName = templateName.(string)
			sinchData.TemplateName = templateName.(string)
		}
		if imageUrl, exists := record["image_url"]; exists && imageUrl != nil {
			timeData.ImageUrl = imageUrl.(string)
		}
		if imageId, exists := record["image_id"]; exists && imageId != nil {
			sinchData.ImageID = imageId.(string)
		}
		if buttonLink, exists := record["link"]; exists && buttonLink != nil {
			timeData.ButtonLink = buttonLink.(string)
			sinchData.ButtonLink = buttonLink.(string)
		}
	}

	// Hit Into WP
	switch msg.Vendor {
	case variables.TIMES:
		timeResponse := timesWhatsapp.HitTimesWhatsappApi(timeData)
		if timeResponse.StatusCode == 200 {
			utils.Info(fmt.Sprintf("WP sent successfully for: %s", msg.Mobile))
			return sdkModels.CommApiResponseBody{
				CommId:  "TimesId: Amartya",
				Success: true,
			}, nil
		}

	case variables.SINCH:
		sinchResponse := sinchWhatsapp.HitSinchApi(sinchData)
		if sinchResponse.StatusCode == 200 {
			utils.Info(fmt.Sprintf("WP sent successfully for: %s", msg.Mobile))
			return sdkModels.CommApiResponseBody{
				CommId:  "SinchId: A.Dey",
				Success: true,
			}, nil
		}
	}
	return sdkModels.CommApiResponseBody{Success: false}, fmt.Errorf("failed to send message for source: %s", msg.Vendor)
}
