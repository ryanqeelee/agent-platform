package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestEnterpriseAdministrationQueueRequiresKnowledgeAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	enforce := true
	g := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforce}}}
	RegisterEnterpriseAdministrationRoutes(
		r.Group("/api/v1"),
		&handler.EnterpriseAdministrationHandler{},
		g,
	)

	w := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/enterprise-administration/queue", nil)
	request = request.WithContext(context.WithValue(
		request.Context(), types.TenantRoleContextKey, types.TenantRoleViewer,
	))
	r.ServeHTTP(w, request)
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee reached enterprise administration queue: status=%d", w.Code)
	}
}
