package rcs_test

import (
	"testing"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/internal/channels/rcs"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestApplyRcsResponseDefaultsWhenVendorSkipped(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		CommId: "C-100",
		Mobile: "9999999999",
		Vendor: "SINCH",
		Client: "wecredit",
		Channel: "RCS",
	}

	response := extapimodels.RcsResponse{}
	rcs.ApplyRcsResponseDefaults(&response, msg, false, "TPL_RCS_1")

	if response.ResponseMessage == "" {
		t.Fatal("ResponseMessage was empty when vendor was skipped")
	}
	if response.ResponseMessage != "shouldHitVendor is off for mobile 9999999999" {
		t.Fatalf("ResponseMessage = %q, want vendor-skipped message", response.ResponseMessage)
	}
	if response.TemplateName != "TPL_RCS_1" {
		t.Fatalf("TemplateName = %q, want TPL_RCS_1", response.TemplateName)
	}
}

func TestApplyRcsResponseDefaultsUsesProviderNoResponseMessage(t *testing.T) {
	msg := sdkModels.CommApiRequestBody{
		CommId: "C-101",
		Mobile: "8888888888",
		Vendor: "PINNACLE",
		Client: "zapcash",
		Channel: "RCS",
	}

	response := extapimodels.RcsResponse{}
	rcs.ApplyRcsResponseDefaults(&response, msg, true, "TPL_RCS_2")

	if response.ResponseMessage != "RCS provider returned no response message for mobile 8888888888" {
		t.Fatalf("ResponseMessage = %q, want provider no-response fallback", response.ResponseMessage)
	}
}
