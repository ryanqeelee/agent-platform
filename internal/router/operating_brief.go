package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterOperatingBriefRoutes(r *gin.RouterGroup, h *handler.OperatingBriefHandler, g *rbacGuards) {
	routes := r.Group("", g.Viewer())
	routes.GET("/operating-analysis-availability", h.Availability)
	routes.GET("/operating-brief", h.Get)
	routes.POST("/operating-brief/refresh", h.Refresh)
	routes.POST("/operating-brief/analysis-handoff", h.CreateHandoff)
}
