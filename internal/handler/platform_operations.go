package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxPlatformOperationsRequestBytes = 1 << 20

var operationsOpaqueID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type PlatformOperationsHandler struct {
	tenants          interfaces.TenantService
	activations      interfaces.EnterpriseActivationService
	tenantOperations interfaces.PlatformOperationsTenantRepository
	members          interfaces.TenantMemberService
	users            interfaces.UserService
	identities       interfaces.PlatformOperationsIdentityService
	bridge           interfaces.PlatformOperationsBridge
	capabilityPlans  *service.CapabilityPlanService
	edgeBindings     interfaces.GovernedEdgeBindingService
}

type enterpriseActivationResponse struct {
	Schema                     string  `json:"schema"`
	ActivationID               string  `json:"activationId"`
	EnterpriseID               string  `json:"enterpriseId"`
	ProductBaseTenantID        *string `json:"productBaseTenantId"`
	BindingID                  *string `json:"bindingId"`
	Status                     string  `json:"status"`
	LastErrorCode              *string `json:"lastErrorCode"`
	Name                       string  `json:"name"`
	Description                string  `json:"description"`
	SeatsTotal                 int     `json:"seatsTotal"`
	StorageQuota               int64   `json:"storageQuota"`
	InitialAdministratorUserID string  `json:"initialAdministratorUserId"`
	AICapabilityPlanVersionID  string  `json:"aiCapabilityPlanVersionId"`
	CreatedAt                  string  `json:"createdAt"`
	UpdatedAt                  string  `json:"updatedAt"`
	CompletedAt                *string `json:"completedAt"`
}

type enterpriseActivationRequest struct {
	Name                       string `json:"name"`
	Description                string `json:"description"`
	SeatsTotal                 int    `json:"seats_total"`
	StorageQuota               int64  `json:"storage_quota"`
	InitialAdministratorUserID string `json:"initial_administrator_user_id"`
}

type enterpriseEdgeSummaryResponse struct {
	ConnectionStatus string  `json:"connectionStatus"`
	NodeCount        int     `json:"nodeCount"`
	OnlineNodeCount  int     `json:"onlineNodeCount"`
	LastSeenAt       *string `json:"lastSeenAt"`
}

type enterpriseEdgeNodeResponse struct {
	EdgeNodeID        string         `json:"edgeNodeId"`
	DisplayName       string         `json:"displayName"`
	Version           string         `json:"version"`
	Status            string         `json:"status"`
	CatalogVersion    *string        `json:"catalogVersion"`
	DataServiceStatus map[string]any `json:"dataServiceStatus"`
	RegisteredAt      *string        `json:"registeredAt"`
	LastSeenAt        *string        `json:"lastSeenAt"`
	ControlRevision   int64          `json:"controlRevision"`
}

type enterpriseEdgeResponse struct {
	Schema              string                        `json:"schema"`
	ProductBaseTenantID string                        `json:"productBaseTenantId"`
	EnterpriseID        string                        `json:"enterpriseId"`
	BindingID           string                        `json:"bindingId"`
	Binding             *types.GovernedEdgeBinding    `json:"binding"`
	Summary             enterpriseEdgeSummaryResponse `json:"summary"`
	Nodes               []enterpriseEdgeNodeResponse  `json:"nodes"`
}

type enterpriseEnrollmentResponse struct {
	Schema              string  `json:"schema"`
	ProductBaseTenantID string  `json:"productBaseTenantId"`
	EnterpriseID        string  `json:"enterpriseId"`
	BindingID           string  `json:"bindingId"`
	EnrollmentToken     string  `json:"enrollmentToken"`
	RotatedAt           *string `json:"rotatedAt"`
}

func NewPlatformOperationsHandler(
	tenants interfaces.TenantService,
	activations interfaces.EnterpriseActivationService,
	tenantOperations interfaces.PlatformOperationsTenantRepository,
	members interfaces.TenantMemberService,
	users interfaces.UserService,
	identities interfaces.PlatformOperationsIdentityService,
	bridge interfaces.PlatformOperationsBridge,
	capabilityPlans *service.CapabilityPlanService,
	edgeBindings interfaces.GovernedEdgeBindingService,
) *PlatformOperationsHandler {
	return &PlatformOperationsHandler{
		tenants: tenants, activations: activations, tenantOperations: tenantOperations, members: members,
		users: users, identities: identities, bridge: bridge,
		capabilityPlans: capabilityPlans, edgeBindings: edgeBindings,
	}
}

func parseOperationsTenantID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("tenant_id")), 10, 64)
	if err != nil || id == 0 {
		c.Error(apperrors.NewValidationError("tenant_id must be a positive integer"))
		return 0, false
	}
	return id, true
}

func activeSeatUsage(members []*types.TenantMember) int64 {
	var used int64
	for _, member := range members {
		if member != nil && member.Status == types.TenantMemberStatusActive {
			used++
		}
	}
	return used
}

func enterpriseProjection(tenant *types.Tenant, seatsUsed int64) gin.H {
	return gin.H{
		"id": tenant.ID, "name": tenant.Name, "description": tenant.Description,
		"status": tenant.Status, "seats_total": tenant.SeatsTotal, "seats_used": seatsUsed,
		"analysis_enabled": tenant.AnalysisEnabled,
		"storage_quota":    tenant.StorageQuota, "storage_used": tenant.StorageUsed,
		"created_at": tenant.CreatedAt, "updated_at": tenant.UpdatedAt,
	}
}

func (h *PlatformOperationsHandler) ListEnterprises(c *gin.Context) {
	tenants, err := h.tenants.ListAllTenants(c.Request.Context())
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list enterprises"))
		return
	}
	items := make([]gin.H, 0, len(tenants))
	for _, tenant := range tenants {
		members, memberErr := h.members.ListByTenant(c.Request.Context(), tenant.ID)
		if memberErr != nil {
			c.Error(apperrors.NewInternalServerError("failed to count enterprise members"))
			return
		}
		items = append(items, enterpriseProjection(tenant, activeSeatUsage(members)))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "total": len(items)}})
}

func (h *PlatformOperationsHandler) GetEnterprise(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	tenant, err := h.tenants.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		if errors.Is(err, apprepo.ErrTenantNotFound) {
			c.Error(apperrors.NewNotFoundError("enterprise not found"))
		} else {
			c.Error(apperrors.NewInternalServerError("failed to get enterprise"))
		}
		return
	}
	members, err := h.members.ListByTenant(c.Request.Context(), tenantID)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to count enterprise members"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": enterpriseProjection(tenant, activeSeatUsage(members))})
}

func (h *PlatformOperationsHandler) UpdateEnterprise(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	var req struct {
		Name            string          `json:"name"`
		Description     string          `json:"description"`
		Status          string          `json:"status"`
		AnalysisEnabled *bool           `json:"analysis_enabled"`
		SeatsTotal      json.RawMessage `json:"seats_total"`
		StorageQuota    *int64          `json:"storage_quota"`
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxPlatformOperationsRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		c.Error(apperrors.NewValidationError("invalid enterprise update"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 128 || len(req.Description) > 512 ||
		(req.Status != types.TenantStatusActive && req.Status != types.TenantStatusSuspended) ||
		req.AnalysisEnabled == nil || len(req.SeatsTotal) == 0 || req.StorageQuota == nil || *req.StorageQuota < 0 {
		c.Error(apperrors.NewValidationError("invalid enterprise update"))
		return
	}
	var seatsTotal *int
	if string(req.SeatsTotal) != "null" {
		var seats int
		if err := json.Unmarshal(req.SeatsTotal, &seats); err != nil || seats < 1 {
			c.Error(apperrors.NewValidationError("seats_total must be positive or null"))
			return
		}
		seatsTotal = &seats
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	tenant, used, err := h.tenantOperations.UpdateForPlatformOperations(
		c.Request.Context(), actorUserID, tenantID, req.Name, req.Description, req.Status,
		*req.AnalysisEnabled, seatsTotal, *req.StorageQuota)
	if err != nil {
		if errors.Is(err, apprepo.ErrSeatLimitBelowUsage) {
			c.Error(apperrors.NewConflictError("seats_total cannot be lower than active member count"))
		} else if errors.Is(err, apprepo.ErrEnterpriseStatusImmutable) {
			c.Error(apperrors.NewConflictError("provisioning or abandoned enterprise status cannot be changed here"))
		} else if errors.Is(err, apprepo.ErrMemberActionForbidden) {
			c.Error(apperrors.NewForbiddenError("system administrator authority is no longer active"))
		} else if errors.Is(err, apprepo.ErrTenantNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			c.Error(apperrors.NewNotFoundError("enterprise not found"))
		} else {
			c.Error(apperrors.NewInternalServerError("failed to update enterprise"))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": enterpriseProjection(tenant, used)})
}

func ordinaryEnterpriseUser(user *types.User, tenantID uint64) bool {
	return user != nil && user.TenantID == tenantID && user.IsActive && !user.IsSystemAdmin &&
		!user.CanAccessAllTenants && !types.IsSyntheticUserID(user.ID)
}

func (h *PlatformOperationsHandler) ListMembers(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	page, pageSize := 1, 50
	if parsed, err := strconv.Atoi(c.Query("page")); err == nil && parsed > 0 {
		page = parsed
	}
	if parsed, err := strconv.Atoi(c.Query("page_size")); err == nil && parsed > 0 && parsed <= 100 {
		pageSize = parsed
	}
	members, total, err := h.members.ListMembersPage(c.Request.Context(), tenantID, strings.TrimSpace(c.Query("q")), page, pageSize)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to list enterprise members"))
		return
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	users, err := h.users.GetUsersByIDs(c.Request.Context(), ids)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to load enterprise members"))
		return
	}
	items := make([]types.TenantMemberResponse, 0, len(members))
	for _, member := range members {
		row := types.TenantMemberResponse{UserID: member.UserID, Role: member.Role, Status: member.Status, InvitedBy: member.InvitedBy, JoinedAt: member.JoinedAt}
		if user := users[member.UserID]; user != nil {
			row.Email, row.Username, row.Avatar = user.Email, user.Username, user.Avatar
		}
		items = append(items, row)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page, "page_size": pageSize}})
}

func (h *PlatformOperationsHandler) CreateEmployee(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	var req types.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("username, email and password are required"))
		return
	}
	user, member, err := h.users.CreateEnterpriseEmployee(c.Request.Context(), tenantID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSeatLimitExceeded), errors.Is(err, service.ErrEnterpriseNotActive), errors.Is(err, service.ErrUserIdentityConflict):
			c.Error(apperrors.NewConflictError(err.Error()))
		case errors.Is(err, service.ErrPasswordPolicy), errors.Is(err, service.ErrComplexPasswordPolicy):
			c.Error(apperrors.NewValidationError(err.Error()))
		default:
			c.Error(apperrors.NewInternalServerError("failed to create employee"))
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"user_id": user.ID, "username": user.Username, "email": user.Email, "role": member.Role, "status": member.Status}})
}

func (h *PlatformOperationsHandler) loadManagedUser(c *gin.Context, tenantID uint64) (*types.User, bool) {
	userID := strings.TrimSpace(c.Param("user_id"))
	user, err := h.users.GetUserByID(c.Request.Context(), userID)
	if err != nil || !ordinaryEnterpriseUser(user, tenantID) {
		c.Error(apperrors.NewForbiddenError("target is not a manageable enterprise user"))
		return nil, false
	}
	return user, true
}

func writeOperationsMemberMutationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrSeatLimitExceeded), errors.Is(err, service.ErrLastAdministrator), errors.Is(err, service.ErrEnterpriseNotActive):
		c.Error(apperrors.NewConflictError(err.Error()))
	case errors.Is(err, service.ErrMembershipNotFound):
		c.Error(apperrors.NewNotFoundError(err.Error()))
	case errors.Is(err, service.ErrInvalidTenantRole), errors.Is(err, service.ErrInvalidMemberStatus):
		c.Error(apperrors.NewValidationError(err.Error()))
	case errors.Is(err, service.ErrMemberActionForbidden), errors.Is(err, service.ErrCannotManageSelf):
		c.Error(apperrors.NewForbiddenError(err.Error()))
	default:
		c.Error(apperrors.NewInternalServerError("failed to update enterprise member"))
	}
}

func (h *PlatformOperationsHandler) UpdateMemberRole(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	if _, ok = h.loadManagedUser(c, tenantID); !ok {
		return
	}
	var req struct {
		Role types.TenantRole `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Role.IsValid() {
		c.Error(apperrors.NewValidationError("role must be admin or viewer"))
		return
	}
	if err := h.members.UpdateRole(c.Request.Context(), c.Param("user_id"), tenantID, req.Role); err != nil {
		writeOperationsMemberMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": true}})
}

func (h *PlatformOperationsHandler) UpdateMemberStatus(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	if _, ok = h.loadManagedUser(c, tenantID); !ok {
		return
	}
	var req struct {
		Status types.TenantMemberStatus `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("status is required"))
		return
	}
	if err := h.members.UpdateStatus(c.Request.Context(), c.Param("user_id"), tenantID, req.Status); err != nil {
		writeOperationsMemberMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": true}})
}

func (h *PlatformOperationsHandler) ResetMemberPassword(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	callerID, _ := types.UserIDFromContext(c.Request.Context())
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("new_password is required"))
		return
	}
	if err := h.identities.ResetEnterpriseMemberPassword(
		c.Request.Context(), callerID, tenantID, strings.TrimSpace(c.Param("user_id")), req.NewPassword,
	); err != nil {
		switch {
		case errors.Is(err, service.ErrPasswordPolicy), errors.Is(err, service.ErrComplexPasswordPolicy):
			c.Error(apperrors.NewValidationError(err.Error()))
		case errors.Is(err, service.ErrPlatformOperationTargetForbidden), errors.Is(err, service.ErrMemberActionForbidden):
			c.Error(apperrors.NewForbiddenError("target is not a manageable enterprise user"))
		case errors.Is(err, service.ErrEnterpriseNotActive):
			c.Error(apperrors.NewConflictError(err.Error()))
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.Error(apperrors.NewNotFoundError("enterprise not found"))
		default:
			c.Error(apperrors.NewInternalServerError("failed to reset password"))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": true}})
}

func (h *PlatformOperationsHandler) CreateInitialAdministrator(c *gin.Context) {
	commandID := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if !operationsOpaqueID.MatchString(commandID) {
		c.Error(apperrors.NewValidationError("Idempotency-Key is required and must be stable"))
		return
	}
	var req types.AdminCreateUserRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxPlatformOperationsRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(&struct{}{}) != io.EOF || req.Password == nil {
		c.Error(apperrors.NewValidationError("username, email and password are required"))
		return
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	result, err := h.identities.CreateInitialAdministrator(c.Request.Context(), actorUserID, commandID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlatformOperationConflict):
			c.Error(apperrors.NewConflictError(err.Error()))
		case errors.Is(err, service.ErrPasswordPolicy), errors.Is(err, service.ErrComplexPasswordPolicy):
			c.Error(apperrors.NewValidationError(err.Error()))
		case errors.Is(err, service.ErrMemberActionForbidden):
			c.Error(apperrors.NewForbiddenError("system administrator authority is no longer active"))
		default:
			c.Error(apperrors.NewInternalServerError("failed to create initial administrator"))
		}
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

func (h *PlatformOperationsHandler) GetInitialAdministrator(c *gin.Context) {
	commandID := strings.TrimSpace(c.Param("command_id"))
	if !operationsOpaqueID.MatchString(commandID) {
		c.Error(apperrors.NewValidationError("command_id is invalid"))
		return
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	result, err := h.identities.GetInitialAdministrator(c.Request.Context(), actorUserID, commandID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlatformOperationNotFound):
			c.Error(apperrors.NewNotFoundError("initial administrator command not found"))
		case errors.Is(err, service.ErrPlatformOperationConflict):
			c.Error(apperrors.NewConflictError(err.Error()))
		case errors.Is(err, service.ErrMemberActionForbidden):
			c.Error(apperrors.NewForbiddenError("system administrator authority is no longer active"))
		default:
			c.Error(apperrors.NewInternalServerError("failed to get initial administrator command"))
		}
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *PlatformOperationsHandler) ProxyEnterpriseActivation(c *gin.Context) {
	activationID := strings.TrimSpace(c.Param("activation_id"))
	if !operationsOpaqueID.MatchString(activationID) {
		c.Error(apperrors.NewValidationError("activation_id is invalid"))
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if !operationsOpaqueID.MatchString(idempotencyKey) {
		c.Error(apperrors.NewValidationError("Idempotency-Key is required and must be stable"))
		return
	}
	var request enterpriseActivationRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxPlatformOperationsRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		strings.TrimSpace(request.Name) == "" || request.Name != strings.TrimSpace(request.Name) || len(request.Name) > 128 ||
		request.Description != strings.TrimSpace(request.Description) || len(request.Description) > 512 ||
		request.SeatsTotal < 1 || request.StorageQuota < 0 || !operationsOpaqueID.MatchString(request.InitialAdministratorUserID) {
		c.Error(apperrors.NewValidationError("invalid enterprise activation"))
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	if err := h.tenantOperations.ValidateSystemAdministrator(c.Request.Context(), actorID); err != nil {
		writeOperationsActivationError(c, err)
		return
	}
	existing, existingErr := h.activations.GetEnterpriseActivation(c.Request.Context(), activationID)
	planVersionID := ""
	if existingErr == nil && existing != nil {
		planVersionID = existing.AICapabilityPlanVersionID
	} else if errors.Is(existingErr, apprepo.ErrTenantNotFound) {
		if h.capabilityPlans == nil {
			c.Error(apperrors.NewConflictError("default capability plan is unavailable"))
			return
		}
		var planErr error
		planVersionID, planErr = h.capabilityPlans.ResolveActivationPlanVersion(c.Request.Context(), actorID)
		if planErr != nil {
			c.Error(apperrors.NewConflictError("default capability plan is unavailable"))
			return
		}
	} else if existingErr != nil {
		c.Error(apperrors.NewInternalServerError("failed to read enterprise activation"))
		return
	}
	canonicalBytes := canonicalEnterpriseActivationV2(request, planVersionID)
	requestDigest := sha256.Sum256(canonicalBytes)
	idempotencyDigest := sha256.Sum256([]byte(idempotencyKey))
	seats, quota := request.SeatsTotal, request.StorageQuota
	command := interfaces.EnterpriseActivationCommand{
		ActivationID: activationID, ActorUserID: actorID,
		IdempotencyKeySHA256: fmt.Sprintf("%x", idempotencyDigest),
		RequestSHA256:        fmt.Sprintf("%x", requestDigest), AICapabilityPlanVersionID: planVersionID,
		TenantName: request.Name, TenantDescription: request.Description,
		FirstOwnerUserID: request.InitialAdministratorUserID,
		DesiredState:     types.EnterpriseActivationStatePrepared, SeatsTotal: &seats, StorageQuota: &quota,
	}
	if _, err := h.activations.ApplyEnterpriseActivation(c.Request.Context(), command); err != nil {
		writeOperationsActivationError(c, err)
		return
	}
	command.DesiredState = types.EnterpriseActivationStateActive
	result, err := h.activations.ApplyEnterpriseActivation(c.Request.Context(), command)
	if err != nil {
		writeOperationsActivationError(c, err)
		return
	}
	writeEnterpriseActivation(c, result)
}

func (h *PlatformOperationsHandler) GetEnterpriseActivation(c *gin.Context) {
	activationID := strings.TrimSpace(c.Param("activation_id"))
	if !operationsOpaqueID.MatchString(activationID) {
		c.Error(apperrors.NewValidationError("activation_id is invalid"))
		return
	}
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	if err := h.tenantOperations.ValidateSystemAdministrator(c.Request.Context(), actorID); err != nil {
		writeOperationsActivationError(c, err)
		return
	}
	result, err := h.activations.GetEnterpriseActivation(c.Request.Context(), activationID)
	if err != nil {
		writeOperationsActivationError(c, err)
		return
	}
	writeEnterpriseActivation(c, result)
}

func writeOperationsActivationError(c *gin.Context, err error) {
	if appErr, ok := apperrors.IsAppError(err); ok {
		c.Error(appErr)
		return
	}
	switch {
	case errors.Is(err, apprepo.ErrTenantNotFound):
		c.Error(apperrors.NewNotFoundError("enterprise activation not found"))
	case errors.Is(err, apprepo.ErrEnterpriseActivationConflict):
		c.Error(apperrors.NewConflictError("enterprise activation conflicts with the durable receipt"))
	case errors.Is(err, apprepo.ErrMemberActionForbidden):
		c.Error(apperrors.NewForbiddenError("system administrator authority is no longer active"))
	default:
		c.Error(err)
	}
}

func canonicalEnterpriseActivationV2(request enterpriseActivationRequest, planVersionID string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(`{"aiCapabilityPlanVersionId":`)
	writeCanonicalJSONString(&buffer, planVersionID)
	buffer.WriteString(`,"description":`)
	writeCanonicalJSONString(&buffer, request.Description)
	buffer.WriteString(`,"initialAdministratorUserId":`)
	writeCanonicalJSONString(&buffer, request.InitialAdministratorUserID)
	buffer.WriteString(`,"name":`)
	writeCanonicalJSONString(&buffer, request.Name)
	buffer.WriteString(`,"schema":"EnterpriseActivationV2","seatsTotal":`)
	buffer.WriteString(strconv.Itoa(request.SeatsTotal))
	buffer.WriteString(`,"storageQuota":`)
	buffer.WriteString(strconv.FormatInt(request.StorageQuota, 10))
	buffer.WriteByte('}')
	return buffer.Bytes()
}

func writeCanonicalJSONString(buffer *bytes.Buffer, value string) {
	buffer.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			buffer.WriteByte('\\')
			buffer.WriteRune(r)
		case '\b':
			buffer.WriteString(`\b`)
		case '\f':
			buffer.WriteString(`\f`)
		case '\n':
			buffer.WriteString(`\n`)
		case '\r':
			buffer.WriteString(`\r`)
		case '\t':
			buffer.WriteString(`\t`)
		default:
			if r < 0x20 {
				buffer.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				buffer.WriteRune(r)
			}
		}
	}
	buffer.WriteByte('"')
}

func writeEnterpriseActivation(c *gin.Context, result *interfaces.EnterpriseActivationResult) {
	tenantID := strconv.FormatUint(result.TenantID, 10)
	bindingID := result.BindingID
	seats := 0
	if result.SeatsTotal != nil {
		seats = *result.SeatsTotal
	}
	response := enterpriseActivationResponse{
		Schema: "EnterpriseActivationV2", ActivationID: result.ActivationID,
		EnterpriseID: result.GovernedEnterpriseID, ProductBaseTenantID: &tenantID,
		BindingID: &bindingID, Status: string(result.State), LastErrorCode: result.LastErrorCode,
		Name: result.Name, Description: result.Description, SeatsTotal: seats,
		StorageQuota: result.StorageQuota, InitialAdministratorUserID: result.InitialOwnerUserID,
		AICapabilityPlanVersionID: result.AICapabilityPlanVersionID,
		CreatedAt:                 result.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: result.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if result.CompletedAt != nil {
		formatted := result.CompletedAt.UTC().Format(time.RFC3339Nano)
		response.CompletedAt = &formatted
	}
	if result.State == types.EnterpriseActivationStateActive {
		response.Status = "completed"
	}
	if result.State == types.EnterpriseActivationStatePrepared {
		response.Status = "pb_prepared"
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func (h *PlatformOperationsHandler) GetEnterpriseEdge(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	if err := h.tenantOperations.ValidateSystemAdministrator(c.Request.Context(), actorUserID); err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	tenant, err := h.tenants.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	if tenant.GovernedEnterpriseID == nil || tenant.GovernedEdgeBinding == nil ||
		tenant.GovernedEdgeBinding.EnterpriseID != *tenant.GovernedEnterpriseID {
		writeOperationsEdgeError(c, apprepo.ErrGovernedEdgeBindingConflict)
		return
	}
	observed, err := h.bridge.GetEnterpriseEdge(c.Request.Context(), *tenant.GovernedEnterpriseID, actorUserID)
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	nodes := make([]enterpriseEdgeNodeResponse, len(observed.Nodes))
	for i, node := range observed.Nodes {
		nodes[i] = enterpriseEdgeNodeResponse(node)
	}
	output := enterpriseEdgeResponse{
		Schema: "PlatformEnterpriseEdgeV1", ProductBaseTenantID: strconv.FormatUint(tenantID, 10),
		EnterpriseID: *tenant.GovernedEnterpriseID, BindingID: tenant.GovernedEdgeBinding.BindingID,
		Binding: tenant.GovernedEdgeBinding,
		Summary: enterpriseEdgeSummaryResponse(observed.Summary), Nodes: nodes,
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": output})
}

func (h *PlatformOperationsHandler) RotateEnterpriseEnrollmentToken(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	if err := h.tenantOperations.ValidateSystemAdministrator(c.Request.Context(), actorUserID); err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	tenant, err := h.tenants.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	if tenant.GovernedEnterpriseID == nil || tenant.GovernedEdgeBinding == nil ||
		tenant.GovernedEdgeBinding.EnterpriseID != *tenant.GovernedEnterpriseID {
		writeOperationsEdgeError(c, apprepo.ErrGovernedEdgeBindingConflict)
		return
	}
	rotated, err := h.bridge.RotateEnterpriseEnrollmentToken(c.Request.Context(), *tenant.GovernedEnterpriseID, actorUserID)
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	output := enterpriseEnrollmentResponse{
		Schema: "PlatformEnterpriseEdgeEnrollmentV1", ProductBaseTenantID: strconv.FormatUint(tenantID, 10),
		EnterpriseID: *tenant.GovernedEnterpriseID, BindingID: tenant.GovernedEdgeBinding.BindingID,
		EnrollmentToken: rotated.EnrollmentToken, RotatedAt: rotated.RotatedAt,
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": output})
}

type edgeBindingCommandRequest struct {
	ExpectedRevision   int64  `json:"expectedRevision"`
	EdgeNodeID         string `json:"edgeNodeId"`
	SourceID           string `json:"sourceId"`
	DeploymentRevision int64  `json:"deploymentRevision"`
}

func readEdgeBindingCommand(c *gin.Context) (edgeBindingCommandRequest, bool) {
	var request edgeBindingCommandRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxPlatformOperationsRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		request.ExpectedRevision < 1 || request.DeploymentRevision < 1 ||
		strings.TrimSpace(request.EdgeNodeID) == "" || request.EdgeNodeID != strings.TrimSpace(request.EdgeNodeID) ||
		strings.TrimSpace(request.SourceID) == "" || request.SourceID != strings.TrimSpace(request.SourceID) {
		c.Error(apperrors.NewValidationError("invalid Edge binding command"))
		return request, false
	}
	return request, true
}

func writeOperationsEdgeError(c *gin.Context, err error) {
	var upstream *interfaces.PlatformOperationsUpstreamError
	switch {
	case errors.Is(err, apprepo.ErrMemberActionForbidden):
		c.Error(apperrors.NewForbiddenError("system administrator authority is no longer active"))
	case errors.Is(err, apprepo.ErrTenantNotFound), errors.Is(err, apprepo.ErrGovernedEnterpriseNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		c.Error(apperrors.NewNotFoundError("enterprise Edge authority was not found"))
	case errors.Is(err, apprepo.ErrGovernedEdgeBindingConflict):
		c.Error(apperrors.NewConflictError("Edge binding changed or does not match the observed node"))
	case errors.As(err, &upstream) && upstream.StatusCode == http.StatusServiceUnavailable && upstream.Detail == "edge_node_disabled_projection_pending":
		c.Error(apperrors.NewServiceUnavailableError("edge_node_disabled_projection_pending"))
	case errors.As(err, &upstream):
		c.Error(apperrors.NewServiceUnavailableError("Center node observation is unavailable"))
	default:
		c.Error(apperrors.NewServiceUnavailableError("Edge binding preflight is unavailable"))
	}
}

func (h *PlatformOperationsHandler) PrepareEnterpriseEdgeBinding(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	request, ok := readEdgeBindingCommand(c)
	if !ok {
		return
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	binding, err := h.edgeBindings.Prepare(c.Request.Context(), interfaces.GovernedEdgeBindingPrepareCommand{
		TenantID: tenantID, ActorUserID: actorUserID, ExpectedRevision: request.ExpectedRevision,
		EdgeNodeID: request.EdgeNodeID, SourceID: request.SourceID, DeploymentRevision: request.DeploymentRevision,
	})
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": binding})
}

func (h *PlatformOperationsHandler) ConfirmEnterpriseEdgeBinding(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	request, ok := readEdgeBindingCommand(c)
	if !ok {
		return
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	binding, err := h.edgeBindings.Confirm(c.Request.Context(), interfaces.GovernedEdgeBindingConfirmCommand{
		TenantID: tenantID, ActorUserID: actorUserID, ExpectedRevision: request.ExpectedRevision,
		EdgeNodeID: request.EdgeNodeID, SourceID: request.SourceID, DeploymentRevision: request.DeploymentRevision,
	})
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": binding})
}

func (h *PlatformOperationsHandler) RevokeEdgeNode(c *gin.Context) {
	var request struct {
		EnterpriseID    string `json:"enterprise_id"`
		EdgeNodeID      string `json:"edge_node_id"`
		ControlRevision int64  `json:"control_revision"`
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxPlatformOperationsRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		request.EnterpriseID == "" || request.EnterpriseID != strings.TrimSpace(request.EnterpriseID) ||
		request.EdgeNodeID == "" || request.EdgeNodeID != strings.TrimSpace(request.EdgeNodeID) ||
		request.ControlRevision < 1 {
		c.Error(apperrors.NewValidationError("invalid Edge node revocation"))
		return
	}
	receipt, err := h.edgeBindings.Revoke(c.Request.Context(), request.EnterpriseID, request.EdgeNodeID, request.ControlRevision)
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"schema":                    "EdgeNodeRevocationReceiptV1",
		"enterprise_id":             receipt.EnterpriseID,
		"edge_node_id":              receipt.EdgeNodeID,
		"sent_control_revision":     receipt.SentControlRevision,
		"accepted_control_revision": receipt.AcceptedControlRevision,
		"binding_revision":          receipt.BindingRevision,
		"status":                    receipt.Status,
	})
}

func (h *PlatformOperationsHandler) DisableEnterpriseEdgeNode(c *gin.Context) {
	tenantID, ok := parseOperationsTenantID(c)
	if !ok {
		return
	}
	actorUserID, _ := types.UserIDFromContext(c.Request.Context())
	if err := h.tenantOperations.ValidateSystemAdministrator(c.Request.Context(), actorUserID); err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	tenant, err := h.tenants.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	if tenant.GovernedEnterpriseID == nil || tenant.GovernedEdgeBinding == nil || tenant.GovernedEdgeBinding.EnterpriseID != *tenant.GovernedEnterpriseID {
		writeOperationsEdgeError(c, apprepo.ErrGovernedEdgeBindingConflict)
		return
	}
	nodeID := strings.TrimSpace(c.Param("node_id"))
	if !operationsOpaqueID.MatchString(nodeID) {
		c.Error(apperrors.NewBadRequestError("invalid edge node id"))
		return
	}
	result, err := h.bridge.DisableEnterpriseEdgeNode(c.Request.Context(), *tenant.GovernedEnterpriseID, nodeID, actorUserID)
	if err != nil {
		writeOperationsEdgeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
