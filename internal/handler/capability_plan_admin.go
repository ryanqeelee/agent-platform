package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxCapabilityPlanAdminRequestBytes = 1 << 20

// CapabilityPlanAdminHandler manages local capability and scenario configuration.
type CapabilityPlanAdminHandler struct {
	service *service.CapabilityPlanService
}

// NewCapabilityPlanAdminHandler constructs the system-admin management handler.
func NewCapabilityPlanAdminHandler(service *service.CapabilityPlanService) *CapabilityPlanAdminHandler {
	return &CapabilityPlanAdminHandler{service: service}
}

func capabilityActorID(c *gin.Context) string {
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	return actorID
}

func parseCapabilityTenantID(c *gin.Context) (uint64, bool) {
	tenantID, err := strconv.ParseUint(strings.TrimSpace(c.Param("tenant_id")), 10, 64)
	if err != nil || tenantID == 0 {
		c.Error(apperrors.NewValidationError("tenant_id must be a positive integer"))
		return 0, false
	}
	return tenantID, true
}

func decodeStrictCapabilityBody(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxCapabilityPlanAdminRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.Error(apperrors.NewValidationError("invalid capability configuration"))
		return false
	}
	return true
}

func writeCapabilityAdminError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCapabilityConfiguration):
		c.Error(apperrors.NewValidationError("invalid capability configuration"))
	case errors.Is(err, repository.ErrCapabilityPlanNotFound):
		c.Error(apperrors.NewNotFoundError("AI capability plan not found"))
	case errors.Is(err, repository.ErrCapabilityTenantNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		c.Error(apperrors.NewNotFoundError("enterprise not found"))
	case errors.Is(err, repository.ErrMemberActionForbidden):
		c.Error(apperrors.NewForbiddenError("system administrator authority is no longer active"))
	case errors.Is(err, interfaces.ErrAICapabilityUnavailable):
		c.Error(apperrors.NewServiceUnavailableError("AI capability plan unavailable"))
	default:
		c.Error(apperrors.NewInternalServerError("failed to manage capability configuration"))
	}
}

type scenarioCapabilityRequest struct {
	ExternalSearch *bool `json:"external_search"`
	MCP            *bool `json:"mcp"`
	Tools          *bool `json:"tools"`
}

func (r scenarioCapabilityRequest) capabilities() (types.AssistantScenarioCapabilities, bool) {
	if r.ExternalSearch == nil || r.MCP == nil || r.Tools == nil {
		return types.AssistantScenarioCapabilities{}, false
	}
	return types.AssistantScenarioCapabilities{ExternalSearch: *r.ExternalSearch, MCP: *r.MCP, Tools: *r.Tools}, true
}

// CreatePlan appends an immutable plan version.
func (h *CapabilityPlanAdminHandler) CreatePlan(c *gin.Context) {
	var input service.CreateCapabilityPlanInput
	if !decodeStrictCapabilityBody(c, &input) {
		return
	}
	plan, err := h.service.CreatePlan(c.Request.Context(), capabilityActorID(c), input)
	if err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusCreated, plan)
}

// ListPlans returns immutable versions and the default pointer.
func (h *CapabilityPlanAdminHandler) ListPlans(c *gin.Context) {
	result, err := h.service.ListPlans(c.Request.Context(), capabilityActorID(c))
	if err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// SetDefaultPlan replaces the singleton platform default pointer.
func (h *CapabilityPlanAdminHandler) SetDefaultPlan(c *gin.Context) {
	var request struct {
		VersionID string `json:"version_id"`
	}
	if !decodeStrictCapabilityBody(c, &request) {
		return
	}
	request.VersionID = strings.TrimSpace(request.VersionID)
	if err := h.service.SetDefaultPlan(c.Request.Context(), capabilityActorID(c), request.VersionID); err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"version_id": request.VersionID})
}

// GetTenantPlan returns the effective plan and source.
func (h *CapabilityPlanAdminHandler) GetTenantPlan(c *gin.Context) {
	tenantID, ok := parseCapabilityTenantID(c)
	if !ok {
		return
	}
	result, err := h.service.GetTenantPlan(c.Request.Context(), capabilityActorID(c), tenantID)
	if err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// AssignTenantPlan sets one tenant override.
func (h *CapabilityPlanAdminHandler) AssignTenantPlan(c *gin.Context) {
	tenantID, ok := parseCapabilityTenantID(c)
	if !ok {
		return
	}
	var request struct {
		VersionID string `json:"version_id"`
	}
	if !decodeStrictCapabilityBody(c, &request) {
		return
	}
	request.VersionID = strings.TrimSpace(request.VersionID)
	if err := h.service.AssignTenantPlan(c.Request.Context(), capabilityActorID(c), tenantID, request.VersionID); err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant_id": tenantID, "version_id": request.VersionID})
}

// ClearTenantPlan restores inheritance from the default.
func (h *CapabilityPlanAdminHandler) ClearTenantPlan(c *gin.Context) {
	tenantID, ok := parseCapabilityTenantID(c)
	if !ok {
		return
	}
	if err := h.service.ClearTenantPlan(c.Request.Context(), capabilityActorID(c), tenantID); err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant_id": tenantID, "inherits_default": true})
}

func scenarioAdminResponse(capabilities types.AssistantScenarioCapabilities, source string) gin.H {
	kind := "platform_shared"
	if source == "tenant_assignment" {
		kind = "enterprise_assigned"
	}
	return gin.H{
		"contract_version": types.AssistantScenarioCapabilityContractVersion,
		"scope":            gin.H{"kind": kind},
		"capabilities":     capabilities,
	}
}

// GetDefaultScenario returns the platform default, or all false when absent.
func (h *CapabilityPlanAdminHandler) GetDefaultScenario(c *gin.Context) {
	result, err := h.service.GetDefaultScenario(c.Request.Context(), capabilityActorID(c))
	if err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, scenarioAdminResponse(result, "platform_default"))
}

// SetDefaultScenario atomically replaces the three platform defaults.
func (h *CapabilityPlanAdminHandler) SetDefaultScenario(c *gin.Context) {
	var request scenarioCapabilityRequest
	if !decodeStrictCapabilityBody(c, &request) {
		return
	}
	capabilities, ok := request.capabilities()
	if !ok {
		writeCapabilityAdminError(c, service.ErrInvalidCapabilityConfiguration)
		return
	}
	if err := h.service.SetDefaultScenario(c.Request.Context(), capabilityActorID(c), capabilities); err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, scenarioAdminResponse(capabilities, "platform_default"))
}

// GetTenantScenario returns the effective policy and source.
func (h *CapabilityPlanAdminHandler) GetTenantScenario(c *gin.Context) {
	tenantID, ok := parseCapabilityTenantID(c)
	if !ok {
		return
	}
	result, err := h.service.GetTenantScenario(c.Request.Context(), capabilityActorID(c), tenantID)
	if err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, scenarioAdminResponse(result.Capabilities, result.Source))
}

// SetTenantScenario atomically replaces the tenant override.
func (h *CapabilityPlanAdminHandler) SetTenantScenario(c *gin.Context) {
	tenantID, ok := parseCapabilityTenantID(c)
	if !ok {
		return
	}
	var request scenarioCapabilityRequest
	if !decodeStrictCapabilityBody(c, &request) {
		return
	}
	capabilities, valid := request.capabilities()
	if !valid {
		writeCapabilityAdminError(c, service.ErrInvalidCapabilityConfiguration)
		return
	}
	if err := h.service.SetTenantScenario(c.Request.Context(), capabilityActorID(c), tenantID, capabilities); err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	c.JSON(http.StatusOK, scenarioAdminResponse(capabilities, "tenant_assignment"))
}

// ClearTenantScenario restores inheritance from the platform default.
func (h *CapabilityPlanAdminHandler) ClearTenantScenario(c *gin.Context) {
	tenantID, ok := parseCapabilityTenantID(c)
	if !ok {
		return
	}
	if err := h.service.ClearTenantScenario(c.Request.Context(), capabilityActorID(c), tenantID); err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	result, err := h.service.ResolveAssistantScenarioCapabilities(c.Request.Context(), tenantID)
	if err != nil {
		writeCapabilityAdminError(c, err)
		return
	}
	source := "platform_default"
	if result.Scope.Kind == "enterprise_assigned" {
		source = "tenant_assignment"
	}
	c.JSON(http.StatusOK, scenarioAdminResponse(result.Capabilities, source))
}
