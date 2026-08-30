package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterEnterpriseAdministrationRoutes(
	r *gin.RouterGroup,
	h *handler.EnterpriseAdministrationHandler,
	g *rbacGuards,
) {
	r.GET("/enterprise-administration/queue", g.Contributor(), h.GetQueue)
}
