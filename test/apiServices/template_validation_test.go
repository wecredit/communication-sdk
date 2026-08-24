package apiServices_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestValidateTemplateStructure(t *testing.T) {
	stage := 1.25

	tests := []struct {
		name    string
		input   apiModels.Templatedetails
		wantErr string
	}{
		{
			name:  "valid stage mode",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V", Stage: &stage},
		},
		{
			name:    "stage mode rejects unsupported channel",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "IVR", Vendor: "V", Stage: &stage},
			wantErr: "unsupported channel",
		},
		{
			name:    "reference sms requires dlt",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "SMS", Vendor: "V"},
			wantErr: "dltTemplateId is required",
		},
		{
			name:  "valid reference whatsapp",
			input: apiModels.Templatedetails{Process: "P", Client: "c", Channel: "WHATSAPP", Vendor: "V", TemplateName: "welcome"},
		},
		{
			name:    "rcs fallback fields are paired",
			input:   apiModels.Templatedetails{Process: "P", Client: "c", Channel: "RCS", Vendor: "V", TemplateName: "offer", DltTemplateId: 1},
			wantErr: "must both be present",
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

func TestTemplateUpdateRequestCanClearStage(t *testing.T) {
	stage := 3.5
	template := apiModels.Templatedetails{Stage: &stage}
	request := apiModels.TemplateUpdateRequest{Stage: apiModels.NullableFloat64{Present: true}}

	request.Apply(&template)
	if template.Stage != nil {
		t.Fatalf("stage was not cleared: %v", *template.Stage)
	}
}
