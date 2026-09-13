package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterPlatformChatHistoryRoutes exposes tenantless platform management for
// the deployment-wide message-index policy and aggregate statistics.
func RegisterPlatformChatHistoryRoutes(
	r *gin.RouterGroup,
	h *handler.PlatformChatHistoryHandler,
	g *rbacGuards,
) {
	if h == nil {
		return
	}
	admin := r.Group("/system/admin", g.SystemAdmin())
	admin.GET("/chat-history-config", h.GetConfig)
	admin.PUT("/chat-history-config", h.UpdateConfig)
	admin.GET("/chat-history-stats", h.GetStats)
}
