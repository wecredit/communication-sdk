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
		WabaNumber:         "waba-1",
		CampaignId:         "4",
		CtaId:              "1",
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
	button, ok := findComponent(components, "button")
	if !ok {
		t.Fatal("utility payload is missing URL button")
	}
	buttonParams, ok := button["parameters"].([]map[string]interface{})
	if !ok || len(buttonParams) != 1 || buttonParams[0]["type"] != "text" || buttonParams[0]["text"] != "cta/waba-1/9876543210/4/1/https://pay.example" {
		t.Fatalf("utility URL button parameter = %#v", button["parameters"])
	}

	payload = pinnacleWhatsappPayload.GetPinnacleUtilityPayload(extapimodels.WhatsappRequestBody{
		Mobile:            "9876543210",
		TemplateName:      "due_date_d_0_utility_01",
		TemplateVariables: "CustomerName,,DueDate",
		CustomerName:      "Asha",
		DueDate:           "2026-10-12",
		ButtonLink:        "https://pay.example",
	})
	template = payload["template"].(map[string]interface{})
	components = template["components"].([]map[string]interface{})
	params = nil
	for _, component := range components {
		if component["type"] == "body" {
			params = component["parameters"].([]map[string]interface{})
		}
	}
	if len(params) != 3 || params[0]["text"] != "Asha" || params[1]["text"] != "" || params[2]["text"] != "2026-10-12" {
		t.Fatalf("empty CSV segment shifted DueDate: %#v", params)
	}
}

func findComponent(components []map[string]interface{}, want string) (map[string]interface{}, bool) {
	for _, component := range components {
		if component["type"] == want {
			return component, true
		}
	}
	return nil, false
}
