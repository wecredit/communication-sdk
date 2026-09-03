package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/handlers"
)

func TestStageConfigurationCreateRejectsUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/stage-configurations", strings.NewReader(`{"lenderName":"x","commType":"SMS","stage":1,"interval":"0d","templateStages":[{"subStage":1}],"id":9}`))
	(&handlers.StageConfigurationHandler{}).Create(context)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestStageConfigurationCreateRejectsAuditFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []string{
		`{"lenderName":"x","commType":"SMS","stage":1,"interval":"0d","createdOn":"2026-08-31T10:00:00Z"}`,
		`{"lenderName":"x","commType":"SMS","stage":1,"interval":"0d","updatedBy":"someone@wecredit.co.in"}`,
	}
	for _, body := range tests {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/stage-configurations", strings.NewReader(body))
		(&handlers.StageConfigurationHandler{}).Create(context)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d for body %s", recorder.Code, body)
		}
	}
}
