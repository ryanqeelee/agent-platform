package handler

import (
	"errors"
	"net/http"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/storageallowlist"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// storageTestErrorMessage returns a safe user-facing message for a storage
// connectivity test failure. Validation/AppError messages are already
// user-facing and are passed through, while raw driver/network errors are
// sanitized so they don't leak internal hostnames, IPs, ports or TLS details.
func storageTestErrorMessage(err error) string {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return sanitizeStorageCheckError(err)
}

type StorageBackendHandler struct {
	repo    interfaces.StorageBackendRepository
	service interfaces.StorageBackendService
	audit   interfaces.AuditLogService
}

func NewStorageBackendHandler(
	repo interfaces.StorageBackendRepository,
	service interfaces.StorageBackendService,
	audit interfaces.AuditLogService,
) *StorageBackendHandler {
	return &StorageBackendHandler{repo: repo, service: service, audit: audit}
}

type storageBackendRequest struct {
	Name     string                     `json:"name" binding:"required"`
	Provider string                     `json:"provider" binding:"required"`
	Config   types.StorageBackendConfig `json:"config"`
	Status   string                     `json:"status,omitempty"`
}

// List godoc
// @Summary      List storage backends
// @Description  List all platform storage backend instances with credentials masked. The platform default backend id is returned alongside the list. System administrator access is required.
// @Tags         StorageBackend
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of storage backends and default_storage_backend_id"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends [get]
func (h *StorageBackendHandler) List(c *gin.Context) {
	backends, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	result := make([]types.StorageBackend, 0, len(backends))
	defaultID := ""
	for _, backend := range backends {
		result = append(result, types.NewStorageBackendResponse(backend))
		if backend.IsDefault {
			defaultID = backend.ID
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result, "default_storage_backend_id": defaultID})
}

// ListCapabilities exposes only the identifiers needed to bind tenant data to
// a platform connection. Credentials and physical locations stay on the
// SystemAdmin surface.
func (h *StorageBackendHandler) ListCapabilities(c *gin.Context) {
	backends, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	type capability struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
	}
	result := make([]capability, 0, len(backends))
	defaultID := ""
	for _, backend := range backends {
		if backend.Status == types.StorageBackendStatusActive {
			result = append(result, capability{ID: backend.ID, Name: backend.Name, Provider: backend.Provider})
			if backend.IsDefault {
				defaultID = backend.ID
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result, "default_storage_backend_id": defaultID})
}

// Get godoc
// @Summary      Get storage backend
// @Description  Retrieve a platform storage backend by ID. Credentials are masked. System administrator access is required.
// @Tags         StorageBackend
// @Produce      json
// @Param        id   path      string  true  "Storage backend ID"
// @Success      200  {object}  map[string]interface{}   "Storage backend details"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  apperrors.AppError          "Storage backend not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends/{id} [get]
func (h *StorageBackendHandler) Get(c *gin.Context) {
	backend, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Error(err)
		return
	}
	if backend == nil {
		c.Error(apperrors.NewNotFoundError("storage backend not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": types.NewStorageBackendResponse(backend)})
}

// Create godoc
// @Summary      Create storage backend
// @Description  Register a platform object/file storage instance. The configuration is validated and a connectivity test is run before the backend is persisted. System administrator access is required.
// @Tags         StorageBackend
// @Accept       json
// @Produce      json
// @Param        request  body      storageBackendRequest    true  "Storage backend configuration"
// @Success      201      {object}  map[string]interface{}   "Created storage backend"
// @Failure      400      {object}  apperrors.AppError          "Invalid request, validation, or connectivity test failure"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      409      {object}  apperrors.AppError          "A storage backend with this name already exists"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends [post]
func (h *StorageBackendHandler) Create(c *gin.Context) {
	var req storageBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	backend := &types.StorageBackend{Name: req.Name, Provider: req.Provider, Config: req.Config, Status: req.Status}
	if err := h.service.Create(c.Request.Context(), backend); err != nil {
		c.Error(err)
		return
	}
	emitPlatformConfigAudit(c.Request.Context(), h.audit, types.AuditActionSystemRetrievalProcessingChanged,
		"create", "storage_backend", backend.ID, "platform_shared",
		platformConfigRevision(backend.CreatedAt, backend.UpdatedAt), []string{"name", "provider", "config", "status"})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": types.NewStorageBackendResponse(backend)})
}

// Update godoc
// @Summary      Update storage backend
// @Description  Update a storage backend's mutable fields (name, credentials, status). Provider and physical location (endpoint, region, bucket, path prefix) are immutable; use storage migration to move data. Redacted secret placeholders preserve the stored credentials.
// @Tags         StorageBackend
// @Accept       json
// @Produce      json
// @Param        id       path      string                   true  "Storage backend ID"
// @Param        request  body      storageBackendRequest    true  "Updated storage backend fields"
// @Success      200      {object}  map[string]interface{}   "Updated storage backend"
// @Failure      400      {object}  apperrors.AppError          "Immutable field change, validation, or connectivity failure"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      404      {object}  apperrors.AppError          "Storage backend not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends/{id} [put]
func (h *StorageBackendHandler) Update(c *gin.Context) {
	var req storageBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	backend := &types.StorageBackend{ID: c.Param("id"), Name: req.Name, Provider: req.Provider, Config: req.Config, Status: req.Status}
	if err := h.service.Update(c.Request.Context(), backend); err != nil {
		c.Error(err)
		return
	}
	updated, err := h.repo.GetByID(c.Request.Context(), backend.ID)
	if err != nil {
		c.Error(err)
		return
	}
	emitPlatformConfigAudit(c.Request.Context(), h.audit, types.AuditActionSystemRetrievalProcessingChanged,
		"update", "storage_backend", updated.ID, "platform_shared",
		platformConfigRevision(updated.CreatedAt, updated.UpdatedAt), []string{"name", "config", "status"})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": types.NewStorageBackendResponse(updated)})
}

// Delete godoc
// @Summary      Delete storage backend
// @Description  Soft-delete a platform storage backend. A backend that is the platform default or still referenced by any knowledge base, active resource, or platform skill archive cannot be deleted.
// @Tags         StorageBackend
// @Produce      json
// @Param        id   path      string  true  "Storage backend ID"
// @Success      200  {object}  map[string]interface{}   "Deletion success"
// @Failure      400  {object}  apperrors.AppError          "Backend is default or bound"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  apperrors.AppError          "Storage backend not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends/{id} [delete]
func (h *StorageBackendHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	backend, err := h.repo.GetByID(ctx, id)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.service.Delete(ctx, id); err != nil {
		c.Error(err)
		return
	}
	if backend != nil {
		emitPlatformConfigAudit(ctx, h.audit, types.AuditActionSystemRetrievalProcessingChanged,
			"delete", "storage_backend", backend.ID, "platform_shared",
			platformConfigRevision(backend.CreatedAt, backend.UpdatedAt), nil)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// SetDefault godoc
// @Summary      Set default storage backend
// @Description  Mark a storage backend as the platform default. Only an active backend can become the default. Knowledge bases without an explicit binding use it.
// @Tags         StorageBackend
// @Produce      json
// @Param        id   path      string  true  "Storage backend ID"
// @Success      200  {object}  map[string]interface{}   "Default set successfully"
// @Failure      400  {object}  apperrors.AppError          "Backend is not active"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  apperrors.AppError          "Storage backend not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends/{id}/default [put]
func (h *StorageBackendHandler) SetDefault(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	backend, err := h.repo.GetByID(ctx, id)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.service.SetDefault(ctx, id); err != nil {
		c.Error(err)
		return
	}
	if backend != nil {
		emitPlatformConfigAudit(ctx, h.audit, types.AuditActionSystemRetrievalProcessingChanged,
			"set_default", "storage_backend", backend.ID, "platform_shared",
			platformConfigRevision(backend.CreatedAt, backend.UpdatedAt), []string{"default"})
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestRaw godoc
// @Summary      Test storage backend with raw config
// @Description  Test connectivity for the provided storage configuration without persisting it. Returns success=false with a sanitized error message on failure (the HTTP status stays 200).
// @Tags         StorageBackend
// @Accept       json
// @Produce      json
// @Param        request  body      storageBackendRequest    true  "Storage backend configuration to test"
// @Success      200      {object}  map[string]interface{}   "Connectivity test result (success, error)"
// @Failure      400      {object}  apperrors.AppError          "Invalid request or validation error"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends/test [post]
func (h *StorageBackendHandler) TestRaw(c *gin.Context) {
	var req storageBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	backend := &types.StorageBackend{Name: req.Name, Provider: req.Provider, Config: req.Config}
	if err := backend.Validate(); err != nil {
		c.Error(err)
		return
	}
	if err := h.service.Test(c.Request.Context(), backend); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": storageTestErrorMessage(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TestByID godoc
// @Summary      Test storage backend by ID
// @Description  Test connectivity of an existing saved storage backend using its stored credentials. Returns success=false with a sanitized error message on failure (the HTTP status stays 200).
// @Tags         StorageBackend
// @Produce      json
// @Param        id   path      string  true  "Storage backend ID"
// @Success      200  {object}  map[string]interface{}   "Connectivity test result (success, error)"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  apperrors.AppError          "Storage backend not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends/{id}/test [post]
func (h *StorageBackendHandler) TestByID(c *gin.Context) {
	backend, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Error(err)
		return
	}
	if backend == nil {
		c.Error(apperrors.NewNotFoundError("storage backend not found"))
		return
	}
	if err := h.service.Test(c.Request.Context(), backend); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": storageTestErrorMessage(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Types godoc
// @Summary      List allowed storage provider types
// @Description  Return the storage provider types allowed by STORAGE_ALLOW_LIST for UI form generation (e.g. local, minio, cos, tos, s3, oss, ks3, obs).
// @Tags         StorageBackend
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of allowed storage provider types"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/storage-backends/types [get]
func (h *StorageBackendHandler) Types(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": storageallowlist.AllowedList()})
}
