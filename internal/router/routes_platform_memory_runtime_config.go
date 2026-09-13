package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterPlatformMemoryRuntimeConfigRoutes exposes deployment-wide memory
// runtime policy only to a tenantless SystemAdmin principal.
func RegisterPlatformMemoryRuntimeConfigRoutes(
	r *gin.RouterGroup, h *handler.PlatformMemoryRuntimeConfigHandler, g *rbacGuards,
) {
	if h == nil {
		return
	}
	admin := r.Group("/system/admin", g.SystemAdmin())
	admin.GET("/memory-runtime-config", h.Get)
	admin.PUT("/memory-runtime-config", h.Update)
}
