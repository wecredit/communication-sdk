package timespayloads

import (
	"fmt"

	models "github.com/wecredit/communication-sdk/sdk/models"
)

// Get Bulk SMS Payload
func GetBulkSmsPayload(recipients []string, config models.Config) (map[string]interface{}, error) {
	shortMessageList := []map[string]string{}

	// Add all recipients to the payload
	for _, mobile := range recipients {
		shortMessageList = append(shortMessageList, map[string]string{
			// "corelationId": fmt.Sprintf("MSG_%d", i+1),
			"dltContentId": config.TimesSmsDltContentId,
			"message":      "Ram Fincorp se lein Rs 1,00,000 tak ka loan bina foreclosure charge ke. Abhi App download karein. https://oow.pw/WCRFIN/viIKwi  WeCredit",
			"recipient":    fmt.Sprintf("91%s", verifyMobile(mobile)),
		})
	}

	payloadData := map[string]interface{}{
		"credentials": map[string]string{
			"user":     config.TimesSmsApiUserName,
			"password": config.TimesSmsApiPassword,
		},
		"from":          config.TimesSmsApiSender,
		"shortMessages": shortMessageList,
		"unicode":       false,
	}

	return payloadData, nil
}
