package consumerServices_test

import (
	"testing"

	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestShouldSubmitZapCashMonitoring(t *testing.T) {
	tests := []struct {
		name     string
		payload  sdkModels.CommApiRequestBody
		accepted bool
		want     bool
	}{
		{name: "accepted sms", payload: sdkModels.CommApiRequestBody{Client: "zapcash", Channel: "SMS"}, accepted: true, want: true},
		{name: "accepted rcs", payload: sdkModels.CommApiRequestBody{Client: "ZapCash", Channel: "rcs"}, accepted: true, want: true},
		{name: "accepted whatsapp", payload: sdkModels.CommApiRequestBody{Client: "zapcash", Channel: "WHATSAPP"}, accepted: true, want: true},
		{name: "provider failed", payload: sdkModels.CommApiRequestBody{Client: "zapcash", Channel: "SMS"}, accepted: false},
		{name: "monitor recursion", payload: sdkModels.CommApiRequestBody{Client: "zapcash", Channel: "SMS", IsMonitorCopy: true}, accepted: true},
		{name: "other client", payload: sdkModels.CommApiRequestBody{Client: "creditsea", Channel: "SMS"}, accepted: true},
		{name: "email", payload: sdkModels.CommApiRequestBody{Client: "zapcash", Channel: "EMAIL"}, accepted: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := services.ShouldSubmitZapCashMonitoring(tc.payload, tc.accepted); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}
