package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/handlers"
	"github.com/wecredit/communication-sdk/internal/middleware"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
)

func TestGetTemplatesRejectsInvalidPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name  string
		query string
	}{
		{name: "zero page", query: "?page=0"},
		{name: "page size above maximum", query: "?pageSize=101"},
		{name: "non numeric page size", query: "?pageSize=all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, recorder := newTemplateContext("/templates/" + tt.query)
			handler := handlers.NewTemplateHandler(nil)

			handler.GetTemplates(context)

			assertTemplateError(t, recorder, http.StatusBadRequest, "INVALID_PAGINATION")
		})
	}
}

func TestGetTemplatesRejectsInvalidStage(t *testing.T) {
	context, recorder := newTemplateContext("/templates/?stage=1.234")
	handler := handlers.NewTemplateHandler(nil)

	handler.GetTemplates(context)

	assertTemplateError(t, recorder, http.StatusBadRequest, "INVALID_STAGE")
}

func TestGetTemplateByIDRejectsInvalidID(t *testing.T) {
	context, recorder := newTemplateContext("/templates/id/not-a-number")
	context.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	handler := handlers.NewTemplateHandler(nil)

	handler.GetTemplateByID(context)

	assertTemplateError(t, recorder, http.StatusBadRequest, "INVALID_TEMPLATE_ID")
}

func TestAddTemplateRejectsNonAllowlistedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
	}{
		{name: "database ID", body: `{"id":123}`},
		{name: "created timestamp", body: `{"createdOn":"2026-08-25T00:00:00Z"}`},
		{name: "updated timestamp", body: `{"updatedOn":"2026-08-25T00:00:00Z"}`},
		{name: "created by", body: `{"createdBy":"someone@wecredit.co.in"}`},
		{name: "updated by", body: `{"updatedBy":"someone@wecredit.co.in"}`},
		{name: "unknown field", body: `{"unexpected":"value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/templates/add-template", strings.NewReader(tt.body))
			context.Request.Header.Set("Content-Type", "application/json")

			handler := handlers.NewTemplateHandler(nil)
			handler.AddTemplate(context)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "unknown field") {
				t.Fatalf("response does not identify the rejected field: %s", recorder.Body.String())
			}
		})
	}
}

func TestAddTemplateRejectsMultipleJSONValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/templates/add-template",
		strings.NewReader(`{"client":"wecredit"} {"client":"other"}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	handler := handlers.NewTemplateHandler(nil)
	handler.AddTemplate(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func newTemplateContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, target, nil)
	middleware.SetCommAdminScope(context, middleware.CommAdminScope{Unrestricted: true})
	return context, recorder
}

func assertTemplateError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, status, recorder.Body.String())
	}
	var response apiModels.TemplateAPIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Success {
		t.Fatal("success = true, want false")
	}
	if response.Error == nil || response.Error.Code != code {
		t.Fatalf("error = %#v, want code %q", response.Error, code)
	}
}
