package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/handlers"
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

func newTemplateContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, target, nil)
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
