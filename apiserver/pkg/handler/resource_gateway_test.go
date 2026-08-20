package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fize/kumquat/apiserver/pkg/service"
	"github.com/gin-gonic/gin"
)

func TestResourceGatewayWorkspaceNamespaceConflictIsHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	(&ResourceGatewayController{}).writeError(ctx, service.ErrWorkspaceNamespaceInUse)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
}

func TestResourceGatewayParentUnavailableIsHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	(&ResourceGatewayController{}).writeError(ctx, service.ErrParentUnavailable)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
}
