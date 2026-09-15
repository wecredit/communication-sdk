package whatsapp_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/whatsapp"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestApplyWhatsappVendorAppIdPrecedenceGoldenPath(t *testing.T) {
	msg := &sdkModels.CommApiRequestBody{
		Vendor: "SINCH",
		AppId:  "payload-app",
	}
	req := &extapimodels.WhatsappRequestBody{AppId: "template-app"}
	whatsapp.ApplyWhatsappVendorAppIdPrecedence(msg, req, "TIMES")
	if msg.Vendor != "TIMES" {
		t.Fatalf("Vendor = %q, want matchedVendor TIMES", msg.Vendor)
	}
	if req.AppId != "template-app" {
		t.Fatalf("AppId = %q, want TemplateDetails-first", req.AppId)
	}

	req2 := &extapimodels.WhatsappRequestBody{AppId: ""}
	msg2 := &sdkModels.CommApiRequestBody{Vendor: "SINCH", AppId: "payload-app"}
	whatsapp.ApplyWhatsappVendorAppIdPrecedence(msg2, req2, "TIMES")
	if req2.AppId != "payload-app" {
		t.Fatalf("empty template AppId fallback = %q", req2.AppId)
	}
}

func TestApplyWhatsappVendorAppIdPrecedenceTrustFlagAtomicPair(t *testing.T) {
	msg := &sdkModels.CommApiRequestBody{
		Vendor:                       "SINCH",
		AppId:                        "nurture-app",
		TrustPayloadWhatsappIdentity: true,
	}
	req := &extapimodels.WhatsappRequestBody{AppId: "template-app"}
	whatsapp.ApplyWhatsappVendorAppIdPrecedence(msg, req, "TIMES")
	if msg.Vendor != "SINCH" {
		t.Fatalf("Vendor overwritten to %q; trust flag must keep payload Vendor", msg.Vendor)
	}
	if req.AppId != "nurture-app" {
		t.Fatalf("AppId = %q, want nurture stamp", req.AppId)
	}
}

func TestApplyWhatsappVendorAppIdPrecedenceTrustFlagEmptyAppId(t *testing.T) {
	msg := &sdkModels.CommApiRequestBody{
		Vendor:                       "SINCH",
		AppId:                        "",
		TrustPayloadWhatsappIdentity: true,
	}
	req := &extapimodels.WhatsappRequestBody{AppId: "template-app"}
	whatsapp.ApplyWhatsappVendorAppIdPrecedence(msg, req, "TIMES")
	if msg.Vendor != "SINCH" {
		t.Fatalf("Vendor = %q", msg.Vendor)
	}
	if req.AppId != "template-app" {
		t.Fatalf("empty nurture AppId should leave TemplateDetails AppId, got %q", req.AppId)
	}
}
