package apiModels_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestTemplateListItemAlwaysIncludesUpdatedOn(t *testing.T) {
	encoded, err := json.Marshal(apiModels.TemplateListItem{
		Id:        14,
		Client:    "wecredit",
		Channel:   "SMS",
		Process:   "BRANCH",
		Vendor:    "PINNACLE",
		IsActive:  true,
		CreatedOn: mustParseTime(t, "2026-08-31T10:00:00Z"),
		UpdatedOn: mustParseTime(t, "2026-08-31T10:00:00Z"),
		CreatedBy: "creator@wecredit.co.in",
		UpdatedBy: "creator@wecredit.co.in",
	})
	if err != nil {
		t.Fatalf("marshal template list item: %v", err)
	}

	response := string(encoded)
	if !strings.Contains(response, `"updatedOn":"2026-08-31T10:00:00Z"`) {
		t.Fatalf("list response missing updatedOn: %s", response)
	}
	if !strings.Contains(response, `"createdOn":"2026-08-31T10:00:00Z"`) {
		t.Fatalf("list response missing createdOn: %s", response)
	}
	if !strings.Contains(response, `"createdBy":"creator@wecredit.co.in"`) {
		t.Fatalf("list response missing createdBy: %s", response)
	}
}

func TestTemplateDetailsResponseFallsBackUpdatedOnToCreatedOn(t *testing.T) {
	createdOn := mustParseTime(t, "2026-08-31T10:00:00Z")
	encoded, err := json.Marshal(apiModels.NewTemplateDetailsResponse(&apiModels.Templatedetails{
		Id:        477,
		CreatedOn: createdOn,
		CreatedBy: "creator@wecredit.co.in",
	}))
	if err != nil {
		t.Fatalf("marshal template details response: %v", err)
	}

	response := string(encoded)
	if !strings.Contains(response, `"updatedOn":"2026-08-31T10:00:00Z"`) {
		t.Fatalf("detail response did not fall back updatedOn to createdOn: %s", response)
	}
	if !strings.Contains(response, `"createdBy":"creator@wecredit.co.in"`) {
		t.Fatalf("detail response missing createdBy: %s", response)
	}
}

func mustParseTime(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("parse time %q: %v", raw, err)
	}
	return parsed
}
