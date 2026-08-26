package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestProductBaseDescriptorRequiresSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-System-Admin") == "true" {
			c.Request = c.Request.WithContext(context.WithValue(
				c.Request.Context(), types.SystemAdminContextKey, true,
			))
		}
		c.Next()
	})
	v1 := r.Group("/api/v1")
	RegisterSystemAdminRoutes(v1, &handler.SystemHandler{}, nil, &rbacGuards{})

	denied := httptest.NewRecorder()
	r.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/product-base", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("ordinary user status = %d, want %d", denied.Code, http.StatusForbidden)
	}

	allowed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/product-base", nil)
	req.Header.Set("X-Test-System-Admin", "true")
	r.ServeHTTP(allowed, req)
	if allowed.Code != http.StatusOK {
		t.Fatalf("system admin status = %d, want %d", allowed.Code, http.StatusOK)
	}
}
