package timespayloads

import (
	"fmt"

	models "dev.azure.com/wctec/communication-engine/sdk/internal/models"
)

func GetBulkMessagePayload(recipients []string, config models.Config) (map[string]interface{}, error) {
	recipientList := []map[string]string{}

	// Add all recipients to the payload
	for _, mobile := range recipients {
		recipientList = append(recipientList, map[string]string{
			"mobile": fmt.Sprintf("91%s", verifyMobile(mobile)),
		})
	}

	payloadData := map[string]interface{}{
		"credentials": map[string]string{
			"user":     config.TimesSmsApiUserName,
			"password": config.TimesSmsApiPassword,
		},
		"options": map[string]string{
			"dltContentId": config.TimesSmsDltContentId,
		},
		"from":        config.TimesSmsApiSender,
		"messageText": "Ram Fincorp se lein Rs 1,00,000 tak ka loan bina foreclosure charge ke. Abhi App download karein. https://oow.pw/WCRFIN/viIKwi  WeCredit",
		"recpients":   recipientList,
		"unicode":     false,
	}

	return payloadData, nil
}
