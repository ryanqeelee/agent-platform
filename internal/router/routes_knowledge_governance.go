package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterKnowledgeGovernanceRoutes(r *gin.RouterGroup, h *handler.KnowledgeGovernanceHandler, g *rbacGuards) {
	if h == nil {
		return
	}
	roles := r.Group("/business-roles")
	roles.GET("", g.Admin(), h.ListBusinessRoles)
	roles.POST("", g.Admin(), h.CreateBusinessRole)
	roles.PUT("/:id", g.Admin(), h.UpdateBusinessRole)
	// The member assignment is intentionally in the existing membership tree:
	// it is Admin-only and does not grant Knowledge Administrators lifecycle authority.
	members := r.Group("/tenants/:id/members", g.PathTenantMatch())
	members.PUT("/:user_id/business-roles", g.Admin(), h.ReplaceMemberBusinessRoles)
	access := r.Group("/knowledge-bases/:id/access")
	access.GET("", g.Admin(), g.KBAccessRead("id"), h.GetKnowledgeBaseAccess)
	access.PUT("", g.Admin(), g.KBAccessWrite("id"), h.ReplaceKnowledgeBaseAccess)
}
