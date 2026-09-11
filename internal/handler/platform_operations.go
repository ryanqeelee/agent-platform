package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

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
	tenantOperations interfaces.PlatformOperationsTenantRepository
	members          interfaces.TenantMemberService
	users            interfaces.UserService
	identities       interfaces.PlatformOperationsIdentityService
	bridge           interfaces.PlatformOperationsBridge
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

func NewPlatformOperationsHandler(
	tenants interfaces.TenantService,
	tenantOperations interfaces.PlatformOperationsTenantRepository,
	members interfaces.TenantMemberService,
	users interfaces.UserService,
	identities interfaces.PlatformOperationsIdentityService,
	bridge interfaces.PlatformOperationsBridge,
) *PlatformOperationsHandler {
	return &PlatformOperationsHandler{
		tenants: tenants, tenantOperations: tenantOperations, members: members,
		users: users, identities: identities, bridge: bridge,
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
		"storage_quota": tenant.StorageQuota, "storage_used": tenant.StorageUsed,
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
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		Status       string          `json:"status"`
		SeatsTotal   json.RawMessage `json:"seats_total"`
		StorageQuota *int64          `json:"storage_quota"`
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
		len(req.SeatsTotal) == 0 || req.StorageQuota == nil || *req.StorageQuota < 0 {
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
		c.Request.Context(), actorUserID, tenantID, req.Name, req.Description, req.Status, seatsTotal, *req.StorageQuota)
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

func (h *PlatformOperationsHandler) proxy(c *gin.Context, method, path, idempotencyKey string, body []byte, secretResponse bool, output any) {
	actorID, _ := types.UserIDFromContext(c.Request.Context())
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	response, err := h.bridge.Do(c.Request.Context(), method, path, actorID, idempotencyKey, reader)
	if err != nil {
		c.Error(apperrors.NewServiceUnavailableError("platform operations service unavailable"))
		return
	}
	if secretResponse {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		contentType := response.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		c.Data(response.StatusCode, contentType, response.Body)
		return
	}
	if err := json.Unmarshal(response.Body, output); err != nil {
		c.Error(apperrors.NewServiceUnavailableError("platform operations service returned an invalid response"))
		return
	}
	c.JSON(response.StatusCode, gin.H{"success": true, "data": output})
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
	var request struct {
		Name                       string `json:"name"`
		Description                string `json:"description"`
		SeatsTotal                 int    `json:"seats_total"`
		StorageQuota               int64  `json:"storage_quota"`
		InitialAdministratorUserID string `json:"initial_administrator_user_id"`
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxPlatformOperationsRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		strings.TrimSpace(request.Name) == "" || len(request.Name) > 128 || len(request.Description) > 512 ||
		request.SeatsTotal < 1 || request.StorageQuota < 0 || !operationsOpaqueID.MatchString(request.InitialAdministratorUserID) {
		c.Error(apperrors.NewValidationError("invalid enterprise activation"))
		return
	}
	body, err := json.Marshal(request)
	if err != nil {
		c.Error(apperrors.NewInternalServerError("failed to encode enterprise activation"))
		return
	}
	var output enterpriseActivationResponse
	h.proxy(c, http.MethodPut, "/api/internal/product-base/operations/enterprise-activations/"+url.PathEscape(activationID), idempotencyKey, body, false, &output)
}

func (h *PlatformOperationsHandler) GetEnterpriseActivation(c *gin.Context) {
	activationID := strings.TrimSpace(c.Param("activation_id"))
	if !operationsOpaqueID.MatchString(activationID) {
		c.Error(apperrors.NewValidationError("activation_id is invalid"))
		return
	}
	var output enterpriseActivationResponse
	h.proxy(c, http.MethodGet, "/api/internal/product-base/operations/enterprise-activations/"+url.PathEscape(activationID), "", nil, false, &output)
}
