package pinnaclePayloads_test

import (
	"testing"

	pinnacleWhatsappPayload "github.com/wecredit/communication-sdk/internal/channels/whatsapp/pinnacle/pinnaclePayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func TestGetPinnacleUtilityPayloadKeepsBlankBodySlots(t *testing.T) {
	payload := pinnacleWhatsappPayload.GetPinnacleUtilityPayload(extapimodels.WhatsappRequestBody{
		Mobile:             "9876543210",
		TemplateName:       "due_date_d_0_utility_01",
		TemplateVariables:  "CustomerName,DueDate,TotalPayableAmount",
		CustomerName:       "Asha",
		DueDate:            "2026-10-12",
		TotalPayableAmount: "",
		ButtonLink:         "https://pay.example",
	})

	template, ok := payload["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("template = %#v", payload["template"])
	}
	components, ok := template["components"].([]map[string]interface{})
	if !ok {
		t.Fatalf("components = %#v", template["components"])
	}
	var params []map[string]interface{}
	for _, component := range components {
		if component["type"] != "body" {
			continue
		}
		params, ok = component["parameters"].([]map[string]interface{})
		if !ok {
			t.Fatalf("parameters = %#v", component["parameters"])
		}
	}
	if len(params) != 3 || params[0]["text"] != "Asha" || params[1]["text"] != "2026-10-12" || params[2]["text"] != "" {
		t.Fatalf("blank total payable shifted later placeholders: %#v", params)
	}
}
