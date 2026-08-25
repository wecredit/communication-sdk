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
