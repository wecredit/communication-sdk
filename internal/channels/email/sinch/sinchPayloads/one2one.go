package sinchEmailPayload

import (
	"fmt"
	"strconv"

	"github.com/wecredit/communication-sdk/helper"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func GetTemplatePayload(data extapimodels.EmailRequestBody, config models.Config) (map[string]interface{}, error) {

	templatePayload := map[string]interface{}{
		"subject": data.EmailSubject, // subject of email
		"from": map[string]interface{}{
			"email": data.FromEmail, // from email
			"name":  "CreditSea",    // name to be shown in email
		},
		"reply_to": map[string]interface{}{
			"email": "help@creditsea.com", // reply to email
			"name":  "CreditSea",          // reply to name
		},
		"recipients": []map[string]interface{}{
			{
				"to": []map[string]interface{}{
					{
						"email": data.ToEmail,
						"name":  data.CustomerName,
					},
				},
				"attributes": map[string]interface{}{
					"first_name": data.CustomerName,
					"loan_id":    data.LoanId,
				},
				"unique_arguments": map[string]interface{}{
					"x-apiheader": strconv.Itoa(helper.GenerateRandomID(100000, 999999)),
				},
			},
		},
		"template_id": data.TemplateId,
	}

	fmt.Println("Template Payload: ", templatePayload)

	return templatePayload, nil
}
