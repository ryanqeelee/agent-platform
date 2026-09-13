package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterPlatformAgentRoutes registers the tenantless SystemAdmin control
// plane for built-in agent definitions.
func RegisterPlatformAgentRoutes(
	r *gin.RouterGroup,
	h *handler.PlatformAgentHandler,
	g *rbacGuards,
) {
	agents := r.Group("/system/admin/agents", g.SystemAdmin())
	{
		agents.GET("/placeholders", h.GetPlaceholders)
		agents.GET("/type-presets", h.GetTypePresets)
		agents.GET("/prompt-templates", h.GetPromptTemplates)
		agents.GET("", h.List)
		agents.GET("/:id", h.Get)
		agents.PUT("/:id", h.Update)
	}
}
