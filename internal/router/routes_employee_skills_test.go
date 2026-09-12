package router

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmployeeSkillRoutesSeparateViewerAndAdminAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		role         types.TenantRole
		method, path string
		status       int
	}{
		{types.TenantRoleViewer, "GET", "/employee-assistant/skills", 503},
		{types.TenantRoleViewer, "GET", "/employee-assistant/skills/manage", 403},
		{types.TenantRoleViewer, "PATCH", "/employee-assistant/skills/manage/skill-1", 403},
		{types.TenantRoleAdmin, "GET", "/employee-assistant/skills/manage", 503},
		{types.TenantRoleAdmin, "PATCH", "/employee-assistant/skills/manage/skill-1", 503},
		{types.TenantRoleAdmin, "GET", "/skills", 404},
	} {
		t.Run(string(tc.role)+tc.method+tc.path, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), types.TenantRoleContextKey, tc.role))
				c.Next()
			})
			enforce := true
			RegisterSkillRoutes(r.Group(""), &handler.SkillHandler{}, &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforce}}})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			require.Equal(t, tc.status, w.Code)
		})
	}
}
