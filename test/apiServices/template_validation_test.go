package apiServices_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestValidateTemplateStructure(t *testing.T) {
	stage := 1.25
	stageWithTooManyDecimals := 1.234

	tests := []struct {
		name    string
		input   apiModels.Templatedetails
		wantErr string
	}{
		{
			name:  "valid stage mode",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", Stage: &stage, TemplateCategory: 3},
		},
		{
			name:    "stage mode rejects unsupported channel",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "IVR", Vendor: "V", Stage: &stage},
			wantErr: "unsupported channel",
		},
		{
			name:    "stage mode rejects more than two decimal places",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", Stage: &stageWithTooManyDecimals, TemplateCategory: 3},
			wantErr: "more than two decimal places",
		},
		{
			name:    "reference sms requires dlt",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", TemplateCategory: 3},
			wantErr: "dltTemplateId is required",
		},
		{
			name:  "sms accepts service implicit category",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 3},
		},
		{
			name:  "sms accepts service explicit category",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 4},
		},
		{
			name:    "sms rejects rcs category",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 1},
			wantErr: "3 (service implicit) or 4 (service explicit)",
		},
		{
			name:  "sms accepts matching placeholders and variables",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 3, TemplateText: "Hi {#var#}, pay at {#var#}", TemplateVariables: "CustomerName,PaymentLink"},
		},
		{
			name:    "sms rejects placeholder variable count mismatch",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 3, TemplateText: "Hi {#var#}, pay at {#var#}", TemplateVariables: "CustomerName"},
			wantErr: "contains 1 entries but templateText contains 2",
		},
		{
			name:    "sms rejects variables without placeholders",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 3, TemplateText: "Hello", TemplateVariables: "CustomerName"},
			wantErr: "contains 1 entries but templateText contains 0",
		},
		{
			name:  "sms accepts one general variable",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 3, TemplateText: "Hello {#var#}", TemplateVariables: "var"},
		},
		{
			name:    "sms rejects repeated general variable",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", DltTemplateId: 1, TemplateCategory: 3, TemplateText: "Hello {#var#} {#var#}", TemplateVariables: "var,var"},
			wantErr: "may only be used as the single variable",
		},
		{
			name:  "valid reference whatsapp",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "WHATSAPP", Vendor: "V", TemplateName: "welcome"},
		},
		{
			name:  "valid push template",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "PUSH", Vendor: "FCM", Stage: &stage, TemplateName: "offer", TemplateHeader: "Offer ready", TemplateText: "Open the app"},
		},
		{
			name:    "push requires title",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "PUSH", Vendor: "FCM", Stage: &stage, TemplateName: "offer", TemplateText: "Open the app"},
			wantErr: "templateHeader is required",
		},
		{
			name:    "push requires body",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "PUSH", Vendor: "FCM", Stage: &stage, TemplateName: "offer", TemplateHeader: "Offer ready"},
			wantErr: "templateText is required",
		},
		{
			name:    "rcs fallback fields are paired",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "V", TemplateName: "offer", DltTemplateId: 1, TemplateCategory: 1},
			wantErr: "must both be present",
		},
		{
			name:    "pinnacle rcs rejects a display name as template identifier",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "PINNACLE", TemplateName: "rcs_payment_reminder_v1", TemplateCategory: 1},
			wantErr: "24-character hexadecimal",
		},
		{
			name:  "pinnacle rcs accepts provider template id",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "PINNACLE", TemplateName: "6a59d83ad71b67a8f6d8030a", TemplateCategory: 1},
		},
		{
			name:  "rcs accepts promotional category",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "V", TemplateName: "offer", TemplateCategory: 2},
		},
		{
			name:    "rcs rejects sms category",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "V", TemplateName: "offer", TemplateCategory: 3},
			wantErr: "1 (transactional) or 2 (promotional)",
		},
		{
			name:  "rcs accepts matching placeholders and variables",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "V", TemplateName: "offer", TemplateCategory: 1, TemplateText: "Hello {#var#}", TemplateVariables: "CustomerName"},
		},
		{
			name:    "rcs rejects placeholder variable count mismatch",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "V", TemplateName: "offer", TemplateCategory: 1, TemplateText: "Hello {#var#}", TemplateVariables: "CustomerName,PaymentLink"},
			wantErr: "contains 2 entries but templateText contains 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apiServices.ValidateTemplateStructure(tt.input)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestTemplateCreateRequestPreservesActiveFlag(t *testing.T) {
	request := apiModels.TemplateCreateRequest{
		Client:   "wecredit",
		Channel:  "SMS",
		Process:  "COLLECTION",
		Vendor:   "SINCH",
		IsActive: true,
	}

	template := request.Template()
	if !template.IsActive {
		t.Fatal("active create request was converted to an inactive template")
	}
}

func TestTemplateUpdateRequestCanClearStage(t *testing.T) {
	stage := 3.5
	template := apiModels.Templatedetails{Stage: &stage}
	request := apiModels.TemplateUpdateRequest{Stage: apiModels.NullableFloat64{Present: true}}

	request.Apply(&template)
	if template.Stage != nil {
		t.Fatalf("stage was not cleared: %v", *template.Stage)
	}
}
