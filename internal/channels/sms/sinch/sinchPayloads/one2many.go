package sinchSmsPayload

import (
	"fmt"
	"strconv"

	models "github.com/wecredit/communication-sdk/sdk/models"
)

func GetBulkMessagePayload(recpients []string, config models.Config) (map[string]interface{}, error) {
	messageList := []map[string]string{}

	// Loop through messages to build the payload
	for i, mobile := range recpients {
		messageList = append(messageList, map[string]string{
			"id":       strconv.Itoa(i + 1),
			"msg":      "Ram Fincorp se lein Rs 1,00,000 tak ka loan bina foreclosure charge ke. Abhi App download karein. https://play.google.com/store/apps/details?id=com.ramfincorploan  WeCredit",
			"to":       fmt.Sprintf("91%s", verifyMobile(mobile)),
			"from":     config.SinchSmsApiSender,
			"language": "en",
			"dtm":      config.SinchSmsDltContentId,
			"tc":       "4",
			"s":        "1",
		})
	}

	payloadData := map[string]interface{}{
		"appid":       config.SinchSmsApiAppID,
		"userId":      config.SinchSmsApiUserName,
		"pass":        config.SinchSmsApiPassword,
		"contenttype": "1",
		"intflag":     "false",
		"selfid":      "true",
		"alert":       "1",
		"messages":    messageList,
	}
	return payloadData, nil
}
