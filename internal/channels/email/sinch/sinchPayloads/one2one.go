package sinchEmailPayload

import (
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func GetTemplatePayload(data extapimodels.EmailRequestBody, config models.Config) (map[string]interface{}, error) {

	templatePayload := map[string]interface{}{
		"subject": "One to one Emails", // subject of email
		"from": map[string]interface{}{
			"email": "mail@sinch.com", // from email
			"name":  "Sinch India",    // name to be shown in email
		},
		"reply_to": map[string]interface{}{
			"email": "noreply@example.com", // reply to email
			"name":  "Reply To",            // reply to name
		},
		"recipients": []map[string]interface{}{
			{
				"to": []map[string]interface{}{
					{
						"email": data.Email,
						"name":  data.CustomerName,
					},
				},
				"attributes": map[string]interface{}{
					":fiedl1": "Mr.John",
					":field2": "Delhi",
					":field3": "Planted",
				},
				"unique_arguments": map[string]interface{}{
					"x-apiheader": "SL1589999999999",
				},
			},
		},
		"template_id": data.TemplateName,
		"headers": map[string]interface{}{
			"X-EXAMPLE": "SL-1234",
		},
	}

	return templatePayload, nil
}
