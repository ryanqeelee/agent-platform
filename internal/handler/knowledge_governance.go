package handler

import (
	"errors"
	"net/http"
	"strings"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type KnowledgeGovernanceHandler struct {
	service interfaces.KnowledgeGovernanceService
}

func NewKnowledgeGovernanceHandler(service interfaces.KnowledgeGovernanceService) *KnowledgeGovernanceHandler {
	return &KnowledgeGovernanceHandler{service: service}
}

func currentTenantID(c *gin.Context) (uint64, bool) {
	tenantID, _ := types.TenantIDFromContext(c.Request.Context())
	if tenantID == 0 {
		_ = c.Error(apperrors.NewUnauthorizedError("workspace ID not found"))
		return 0, false
	}
	return tenantID, true
}

func requireSourceTenant(c *gin.Context, sourceTenantID uint64) bool {
	if callerTenantID := c.GetUint64(types.TenantIDContextKey.String()); callerTenantID != 0 && callerTenantID != sourceTenantID {
		_ = c.Error(apperrors.NewForbiddenError("knowledge access is managed only in the source workspace"))
		return false
	}
	return true
}

func governanceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, apprepo.ErrBusinessRoleNotFound), errors.Is(err, apprepo.ErrKnowledgeAccessTargetNotFound):
		_ = c.Error(apperrors.NewNotFoundError("business role or knowledge base not found"))
	case errors.Is(err, apprepo.ErrKnowledgeAccessRoleInvalid):
		_ = c.Error(apperrors.NewValidationError("role IDs must be enabled roles in this workspace"))
	default:
		_ = c.Error(apperrors.NewInternalServerError("knowledge access governance request failed"))
	}
}

func (h *KnowledgeGovernanceHandler) ListBusinessRoles(c *gin.Context) {
	tenantID, ok := currentTenantID(c)
	if !ok {
		return
	}
	roles, err := h.service.ListBusinessRoles(c.Request.Context(), tenantID)
	if err != nil {
		governanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": roles})
}

type businessRoleRequest struct {
	Name    string `json:"name" binding:"required"`
	Enabled *bool  `json:"enabled"`
}

func (h *KnowledgeGovernanceHandler) CreateBusinessRole(c *gin.Context) {
	tenantID, ok := currentTenantID(c)
	if !ok {
		return
	}
	var req businessRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		_ = c.Error(apperrors.NewValidationError("name is required"))
		return
	}
	role, err := h.service.CreateBusinessRole(c.Request.Context(), tenantID, req.Name)
	if err != nil {
		governanceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": role})
}

func (h *KnowledgeGovernanceHandler) UpdateBusinessRole(c *gin.Context) {
	tenantID, ok := currentTenantID(c)
	if !ok {
		return
	}
	var req businessRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" || req.Enabled == nil {
		_ = c.Error(apperrors.NewValidationError("name and enabled are required"))
		return
	}
	role, err := h.service.UpdateBusinessRole(c.Request.Context(), tenantID, strings.TrimSpace(c.Param("id")), req.Name, *req.Enabled)
	if err != nil {
		governanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": role})
}

type memberBusinessRolesRequest struct {
	RoleIDs []string `json:"role_ids"`
}

func (h *KnowledgeGovernanceHandler) ReplaceMemberBusinessRoles(c *gin.Context) {
	tenantID, ok := currentTenantID(c)
	if !ok {
		return
	}
	var req memberBusinessRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewValidationError("invalid request body"))
		return
	}
	if err := h.service.ReplaceMemberBusinessRoles(c.Request.Context(), tenantID, strings.TrimSpace(c.Param("user_id")), req.RoleIDs); err != nil {
		governanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type knowledgeAccessRequest struct {
	Mode    string   `json:"mode" binding:"required"`
	RoleIDs []string `json:"role_ids"`
}

func (h *KnowledgeGovernanceHandler) GetKnowledgeBaseAccess(c *gin.Context) {
	tenantID, ok := currentTenantID(c)
	if !ok {
		return
	}
	if !requireSourceTenant(c, tenantID) {
		return
	}
	roles, err := h.service.GetKnowledgeBaseRoleGrants(c.Request.Context(), tenantID, strings.TrimSpace(c.Param("id")))
	if err != nil {
		governanceError(c, err)
		return
	}
	mode := "roles"
	if len(roles) == 0 {
		mode = "all"
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"mode": mode, "role_ids": roles}})
}

func (h *KnowledgeGovernanceHandler) ReplaceKnowledgeBaseAccess(c *gin.Context) {
	tenantID, ok := currentTenantID(c)
	if !ok {
		return
	}
	if !requireSourceTenant(c, tenantID) {
		return
	}
	var req knowledgeAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewValidationError("invalid request body"))
		return
	}
	if err := h.service.ReplaceKnowledgeBaseRoleGrants(c.Request.Context(), tenantID, strings.TrimSpace(c.Param("id")), req.Mode, req.RoleIDs); err != nil {
		governanceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
