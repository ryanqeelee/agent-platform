package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type OperatingBriefHandler struct {
	brief    *service.OperatingBriefService
	members  interfaces.TenantMemberService
	tenants  interfaces.TenantService
	resolver interfaces.GovernedEdgeResolver
}

func NewOperatingBriefHandler(
	brief *service.OperatingBriefService,
	members interfaces.TenantMemberService,
	tenants interfaces.TenantService,
	resolver interfaces.GovernedEdgeResolver,
) *OperatingBriefHandler {
	return &OperatingBriefHandler{brief: brief, members: members, tenants: tenants, resolver: resolver}
}

func (h *OperatingBriefHandler) Availability(c *gin.Context) {
	ctx := c.Request.Context()
	permission, err := service.CurrentOperatingAnalysisReadPermission(ctx, h.members, h.tenants)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operating analysis availability unavailable"})
		return
	}
	availability := gin.H{
		"state": "hidden", "reasonCode": "enterprise_entitlement_off",
		"canExchange": false, "canReadHistory": false, "nextAction": "none",
	}
	if permission.ProductEnabled && !permission.MemberEnabled {
		availability["state"] = "disabled"
		availability["reasonCode"] = "missing_member_permission"
		availability["nextAction"] = "contact_admin"
	} else if permission.Allowed() {
		availability["canReadHistory"] = true
		tenantID, _ := types.TenantIDFromContext(ctx)
		connection, resolveErr := h.resolver.Resolve(ctx, tenantID)
		if resolveErr == nil {
			availability["state"] = "enabled"
			availability["reasonCode"] = "enabled"
			availability["canExchange"] = true
			availability["nextAction"] = "none"
			availability["revision"] = strconv.FormatInt(connection.Revision, 10)
		} else {
			availability["state"] = "disabled"
			availability["reasonCode"] = "service_unavailable"
			availability["nextAction"] = "service_unavailable"
		}
	}
	c.JSON(http.StatusOK, gin.H{"schema": "OperatingAnalysisAvailabilityV1", "availability": availability})
}

func (h *OperatingBriefHandler) Get(c *gin.Context) {
	result, err := h.brief.Read(c.Request.Context(), strings.TrimSpace(c.Query("scopeRef")))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *OperatingBriefHandler) Refresh(c *gin.Context) {
	if err := h.brief.EnqueueRefresh(c.Request.Context(), strings.TrimSpace(c.Query("scopeRef"))); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func (h *OperatingBriefHandler) CreateHandoff(c *gin.Context) {
	var request struct {
		BriefSnapshotRef     string `json:"briefSnapshotRef" binding:"required"`
		ObservationAnchorRef string `json:"observationAnchorRef" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid operating brief handoff"})
		return
	}
	result, err := h.brief.CreateHandoff(c.Request.Context(), strings.TrimSpace(request.BriefSnapshotRef), strings.TrimSpace(request.ObservationAnchorRef))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *OperatingBriefHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tools.ErrGovernedDataAccessDenied):
		c.JSON(http.StatusForbidden, gin.H{"error": "operating analysis access is not permitted"})
	case errors.Is(err, apprepo.ErrOperatingBriefSnapshotNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "operating brief reference not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operating brief unavailable"})
	}
}
