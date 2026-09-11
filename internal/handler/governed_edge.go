package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func (h *TenantHandler) PutGovernedEdgeBinding(c *gin.Context) {
	scope, ok := types.TenantAPIKeyScopeFromContext(c.Request.Context())
	if !ok || !scope.IsPlatform() {
		c.JSON(http.StatusForbidden, gin.H{"error": "Edge bindings require platform service authentication"})
		return
	}
	tenantID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	var binding types.GovernedEdgeBinding
	if err != nil || tenantID == 0 || c.ShouldBindJSON(&binding) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Edge binding"})
		return
	}
	if err := h.service.ApplyGovernedEdgeBinding(c.Request.Context(), tenantID, binding); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Edge binding could not be applied"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"binding_id": binding.BindingID, "revision": binding.Revision})
}
