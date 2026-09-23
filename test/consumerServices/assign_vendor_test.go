package consumerServices_test

import (
	"testing"

	consumerServices "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func TestAssignVendorEmptyPushDefaultsToFCM(t *testing.T) {
	data := sdkModels.CommApiRequestBody{
		Client:  "zapcash",
		Channel: variables.PUSH,
		CommId:  "comm-1",
	}
	if !consumerServices.AssignVendor(&data) {
		t.Fatal("AssignVendor returned false for empty-vendor PUSH")
	}
	if data.Vendor != variables.FCM {
		t.Fatalf("Vendor = %q, want FCM", data.Vendor)
	}
}

func TestAssignVendorEmptyEmailStillDefaultsToSINCH(t *testing.T) {
	data := sdkModels.CommApiRequestBody{
		Client:  "wecredit",
		Channel: variables.Email,
		CommId:  "comm-2",
	}
	if !consumerServices.AssignVendor(&data) {
		t.Fatal("AssignVendor returned false for empty-vendor Email")
	}
	if data.Vendor != variables.SINCH {
		t.Fatalf("Vendor = %q, want SINCH", data.Vendor)
	}
}
