package sinchEmailPayload

import (
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func GetTemplatePayload(data extapimodels.EmailRequestBody, config models.Config) (map[string]interface{}, error) {

	templatePayload := map[string]interface{}{
		"subject": "One to one Emails",
		"from": map[string]interface{}{
			"email": "mail@sinch.com",
			"name":  "Sinch India",
		},
		"reply_to": map[string]interface{}{
			"email": "noreply@example.com",
			"name":  "Reply To",
		},
		"recipients": []map[string]interface{}{
			{
				"to": []map[string]interface{}{
					{
						"email": "success@simulator.example.com",
						"name":  "John Doe",
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
		"template_id": "twig_tmpl_20210519_unicode_large",
		"headers": map[string]interface{}{
			"X-EXAMPLE": "SL-1234",
		},
	}

	return templatePayload, nil
}
