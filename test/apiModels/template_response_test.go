package apiModels_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
)

func TestTemplateListItemExcludesPayloadFields(t *testing.T) {
	stage := "2.10"
	encoded, err := json.Marshal(apiModels.TemplateListItem{
		Id:           14,
		Client:       "wecredit",
		Channel:      "RCS",
		Process:      "OLYV",
		Stage:        &stage,
		Vendor:       "SINCH",
		TemplateName: "template-name",
		IsActive:     true,
	})
	if err != nil {
		t.Fatalf("marshal template list item: %v", err)
	}

	response := string(encoded)
	if !strings.Contains(response, `"stage":"2.10"`) {
		t.Fatalf("list response does not preserve canonical stage precision: %s", response)
	}
	for _, excluded := range []string{
		"templateText",
		"templateVariables",
		"smsFallbackVariables",
		"imageId",
		"imageUrl",
		"templateEntityId",
		"templateHeader",
		"templateCategory",
		"link",
		"subject",
		"fromEmail",
	} {
		if strings.Contains(response, `"`+excluded+`"`) {
			t.Fatalf("list response unexpectedly contains %q: %s", excluded, response)
		}
	}
}

func TestTemplateDetailsResponsePreservesCanonicalStagePrecision(t *testing.T) {
	stage := 2.10
	encoded, err := json.Marshal(apiModels.NewTemplateDetailsResponse(&apiModels.Templatedetails{
		Id:    477,
		Stage: &stage,
	}))
	if err != nil {
		t.Fatalf("marshal template details response: %v", err)
	}

	response := string(encoded)
	if !strings.Contains(response, `"stage":"2.10"`) {
		t.Fatalf("detail response does not preserve canonical stage precision: %s", response)
	}
	if strings.Count(response, `"stage"`) != 1 {
		t.Fatalf("detail response contains duplicate stage fields: %s", response)
	}
}
