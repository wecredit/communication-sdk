package channelHelper

import (
	"testing"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func TestPopulateSmsFieldsConvertsTemplateEntityID(t *testing.T) {
	request := extapimodels.SmsRequestBody{}
	PopulateSmsFields(&request, map[string]interface{}{
		"TemplateEntityId": int64(1001234567890123456),
		"TemplateHeader":   "WECRPL",
	})

	if request.TemplateEntityId != "1001234567890123456" {
		t.Fatalf("TemplateEntityId = %q", request.TemplateEntityId)
	}
	if request.TemplateHeader != "WECRPL" {
		t.Fatalf("TemplateHeader = %q", request.TemplateHeader)
	}
}
