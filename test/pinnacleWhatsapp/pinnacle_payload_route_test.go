package pinnacleWhatsapp_test

import (
	"testing"

	pinnacleWhatsapp "github.com/wecredit/communication-sdk/internal/channels/whatsapp/pinnacle"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func TestGetPinnaclePayloadRoutesByProcessLikeHermis(t *testing.T) {
	utility, err := pinnacleWhatsapp.GetPinnaclePayload(extapimodels.WhatsappRequestBody{
		Process:      "CASHVIA_UTILITY",
		TemplateName: "w8_cashvia_utility_07_05",
		Mobile:       "7014850582",
		ButtonLink:   "https://example.com/<mobile>",
	})
	if err != nil {
		t.Fatalf("utility process: %v", err)
	}
	if _, hasHeader := componentType(utility, "header"); hasHeader {
		t.Fatalf("utility process must not attach IMAGE header")
	}

	// Marketing template name, non-utility process → media (Hermis parity).
	marketing, err := pinnacleWhatsapp.GetPinnaclePayload(extapimodels.WhatsappRequestBody{
		Process:      "BRANCH",
		TemplateName: "branch_marketing_july_02",
		Mobile:       "7014850582",
		ButtonLink:   "https://branch.co/<mobile>",
		ImageUrl:     "https://whatsappdata.s3.ap-south-1.amazonaws.com/userMedia/branch_marketing.png",
	})
	if err != nil {
		t.Fatalf("marketing process: %v", err)
	}
	if _, hasHeader := componentType(marketing, "header"); !hasHeader {
		t.Fatalf("BRANCH + ImageUrl must attach IMAGE header via media payload")
	}
}

func componentType(payload map[string]interface{}, want string) (map[string]interface{}, bool) {
	tmpl, _ := payload["template"].(map[string]interface{})
	comps, _ := tmpl["components"].([]map[string]interface{})
	for _, c := range comps {
		if c["type"] == want {
			return c, true
		}
	}
	raw, _ := tmpl["components"].([]interface{})
	for _, item := range raw {
		c, _ := item.(map[string]interface{})
		if c["type"] == want {
			return c, true
		}
	}
	return nil, false
}
