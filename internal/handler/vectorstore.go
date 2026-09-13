package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// VectorStoreHandler handles HTTP requests for vector store CRUD
type VectorStoreHandler struct {
	repo    interfaces.VectorStoreRepository
	service interfaces.VectorStoreService
	audit   interfaces.AuditLogService
}

// NewVectorStoreHandler creates a new handler
func NewVectorStoreHandler(
	repo interfaces.VectorStoreRepository,
	service interfaces.VectorStoreService,
	audit interfaces.AuditLogService,
) *VectorStoreHandler {
	return &VectorStoreHandler{repo: repo, service: service, audit: audit}
}

// --- request DTOs ---

// CreateStoreRequest defines the request body for creating a vector store
type CreateStoreRequest struct {
	Name             string                    `json:"name" binding:"required"`
	EngineType       types.RetrieverEngineType `json:"engine_type" binding:"required"`
	ConnectionConfig types.ConnectionConfig    `json:"connection_config" binding:"required"`
	IndexConfig      types.IndexConfig         `json:"index_config"`
}

// UpdateStoreRequest defines the request body for updating a vector store.
// Only name is mutable — engine_type, connection_config, index_config are immutable.
type UpdateStoreRequest struct {
	Name string `json:"name" binding:"required"`
}

// TestStoreRequest defines the body for testing raw credentials
type TestStoreRequest struct {
	EngineType       types.RetrieverEngineType `json:"engine_type" binding:"required"`
	ConnectionConfig types.ConnectionConfig    `json:"connection_config" binding:"required"`
}

// --- helpers ---

// getStore loads a global store by ID.
// Returns (nil, status, msg) on failure so callers can respond immediately.
func (h *VectorStoreHandler) getStore(c *gin.Context, id string) (*types.VectorStore, int, string) {
	store, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		return nil, http.StatusInternalServerError, "failed to query vector store"
	}
	if store == nil {
		return nil, http.StatusNotFound, "vector store not found"
	}
	return store, http.StatusOK, ""
}

// --- endpoints ---

// CreateStore godoc
// @Summary      Create vector store
// @Description  Create a platform-global vector store configuration. System administrator access is required.
// @Tags         VectorStore
// @Accept       json
// @Produce      json
// @Param        request  body      CreateStoreRequest       true  "Vector store configuration"
// @Success      201      {object}  map[string]interface{}   "Created vector store"
// @Failure      400      {object}  errors.AppError          "Invalid request or validation error"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/vector-stores [post]
func (h *VectorStoreHandler) CreateStore(c *gin.Context) {
	ctx := c.Request.Context()

	var req CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warnf(ctx, "Invalid create vector store request: %v", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	store := &types.VectorStore{
		Name:             req.Name,
		EngineType:       req.EngineType,
		ConnectionConfig: req.ConnectionConfig,
		IndexConfig:      req.IndexConfig,
	}

	if err := h.service.CreateStore(ctx, store); err != nil {
		logger.Warnf(ctx, "Failed to create vector store: %v", err)
		c.Error(err)
		return
	}
	emitPlatformConfigAudit(ctx, h.audit, types.AuditActionSystemRetrievalProcessingChanged,
		"create", "vector_store", store.ID, "platform_shared",
		platformConfigRevision(store.CreatedAt, store.UpdatedAt),
		[]string{"name", "engine_type", "connection_config", "index_config"})

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    types.NewVectorStoreResponse(store),
	})
}

// ListStores godoc
// @Summary      List vector stores
// @Description  List all platform-global vector stores with credentials masked. System administrator access is required.
// @Tags         VectorStore
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of vector stores"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/vector-stores [get]
func (h *VectorStoreHandler) ListStores(c *gin.Context) {
	ctx := c.Request.Context()

	dbStores, err := h.repo.List(ctx)
	if err != nil {
		logger.Warnf(ctx, "Failed to list vector stores: %v", err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	// Mask connection secrets in administrator responses.
	maskedDBStores := make([]types.VectorStoreResponse, len(dbStores))
	defaultID := ""
	for i, s := range dbStores {
		maskedDBStores[i] = types.NewVectorStoreResponse(s)
		if s.IsDefault {
			defaultID = s.ID
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": maskedDBStores, "default_vector_store_id": defaultID})
}

func (h *VectorStoreHandler) ListCapabilities(c *gin.Context) {
	stores, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	type capability struct {
		ID         string                    `json:"id"`
		Name       string                    `json:"name"`
		EngineType types.RetrieverEngineType `json:"engine_type"`
	}
	result := make([]capability, 0, len(stores))
	defaultID := ""
	for _, store := range stores {
		result = append(result, capability{ID: store.ID, Name: store.Name, EngineType: store.EngineType})
		if store.IsDefault {
			defaultID = store.ID
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result, "default_vector_store_id": defaultID})
}

// GetStore godoc
// @Summary      Get vector store
// @Description  Retrieve a single platform vector store by ID.
// @Tags         VectorStore
// @Produce      json
// @Param        id   path      string  true  "Vector store ID"
// @Success      200  {object}  map[string]interface{}   "Vector store details"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/vector-stores/{id} [get]
func (h *VectorStoreHandler) GetStore(c *gin.Context) {
	id := c.Param("id")
	store, status, msg := h.getStore(c, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.NewVectorStoreResponse(store),
	})
}

// UpdateStore godoc
// @Summary      Update vector store
// @Description  Update a vector store (name only). Engine type, connection config, and index config are immutable.
// @Tags         VectorStore
// @Accept       json
// @Produce      json
// @Param        id       path      string               true  "Vector store ID"
// @Param        request  body      UpdateStoreRequest   true  "Updated fields"
// @Success      200      {object}  map[string]interface{}   "Updated vector store"
// @Failure      400      {object}  map[string]interface{}   "Validation error"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Failure      404      {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/vector-stores/{id} [put]
func (h *VectorStoreHandler) UpdateStore(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	if _, status, msg := h.getStore(c, id); status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	var req UpdateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	updated := &types.VectorStore{
		ID:   id,
		Name: req.Name,
	}

	if err := h.service.UpdateStore(ctx, updated); err != nil {
		logger.Warnf(ctx, "Failed to update vector store %s: %v", id, err)
		c.Error(err)
		return
	}

	// Re-fetch to return full state
	result, err := h.repo.GetByID(ctx, id)
	if err != nil {
		logger.Warnf(ctx, "Failed to re-fetch vector store %s after update: %v", id, err)
	}
	if result != nil {
		emitPlatformConfigAudit(ctx, h.audit, types.AuditActionSystemRetrievalProcessingChanged,
			"update", "vector_store", result.ID, "platform_shared",
			platformConfigRevision(result.CreatedAt, result.UpdatedAt), []string{"name"})
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    types.NewVectorStoreResponse(result),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
	}
}

// DeleteStore godoc
// @Summary      Delete vector store
// @Description  Soft-delete a platform vector store. The platform default or a store referenced by any knowledge base cannot be deleted.
// @Tags         VectorStore
// @Produce      json
// @Param        id   path      string  true  "Vector store ID"
// @Success      200  {object}  map[string]interface{}   "Deletion success"
// @Failure      400  {object}  map[string]interface{}   "Store is default or referenced"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/vector-stores/{id} [delete]
func (h *VectorStoreHandler) DeleteStore(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	store, status, msg := h.getStore(c, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	if err := h.service.DeleteStore(ctx, id); err != nil {
		logger.Warnf(ctx, "Failed to delete vector store %s: %v", id, err)
		c.Error(err)
		return
	}
	emitPlatformConfigAudit(ctx, h.audit, types.AuditActionSystemRetrievalProcessingChanged,
		"delete", "vector_store", id, "platform_shared",
		platformConfigRevision(store.CreatedAt, store.UpdatedAt), nil)

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *VectorStoreHandler) SetDefaultStore(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	store, status, msg := h.getStore(c, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}
	if err := h.service.SetDefaultStore(ctx, id); err != nil {
		c.Error(err)
		return
	}
	emitPlatformConfigAudit(ctx, h.audit, types.AuditActionSystemRetrievalProcessingChanged,
		"set_default", "vector_store", store.ID, "platform_shared",
		platformConfigRevision(store.CreatedAt, store.UpdatedAt), []string{"default"})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListStoreTypes godoc
// @Summary      List vector store types
// @Description  Return supported engine types with connection and index field schemas for UI form generation
// @Tags         VectorStore
// @Produce      json
// @Success      200  {object}  map[string]interface{}   "List of engine types with config schemas"
// @Router       /system/admin/vector-stores/types [get]
func (h *VectorStoreHandler) ListStoreTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.GetVectorStoreTypes(),
	})
}

// TestStoreByID godoc
// @Summary      Test vector store connection by ID
// @Description  Test connectivity of an existing saved store. Returns the detected server version and saves it to connection_config.
// @Tags         VectorStore
// @Produce      json
// @Param        id   path      string  true  "Vector store ID"
// @Success      200  {object}  map[string]interface{}   "Connection test result (success, version)"
// @Failure      401  {object}  map[string]interface{}   "Unauthorized"
// @Failure      404  {object}  map[string]interface{}   "Vector store not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/vector-stores/{id}/test [post]
func (h *VectorStoreHandler) TestStoreByID(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	store, status, msg := h.getStore(c, id)
	if status != http.StatusOK {
		c.JSON(status, gin.H{"success": false, "error": msg})
		return
	}

	version, err := h.service.TestConnection(ctx, store.EngineType, store.ConnectionConfig)
	if err != nil {
		safeError := sanitizeStorageCheckError(err)
		logger.Warnf(ctx, "Vector store connection test failed: %s", safeError)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": safeError})
		return
	}

	// Update stored version if detected
	if version != "" && version != store.ConnectionConfig.Version {
		if updateErr := h.service.SaveDetectedVersion(ctx, store, version); updateErr != nil {
			logger.Warnf(ctx, "Failed to update detected version for store %s: %s", store.ID, sanitizeStorageCheckError(updateErr))
		} else if updatedStore, loadErr := h.repo.GetByID(ctx, store.ID); loadErr != nil {
			logger.Warnf(ctx, "Failed to load updated vector store %s: %s", store.ID, sanitizeStorageCheckError(loadErr))
		} else if updatedStore != nil {
			emitPlatformConfigAudit(ctx, h.audit, types.AuditActionSystemRetrievalProcessingChanged,
				"detected_version_updated", "vector_store", updatedStore.ID, "platform_shared",
				platformConfigRevision(updatedStore.CreatedAt, updatedStore.UpdatedAt), []string{"connection_config.version"})
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
}

// TestStoreRaw godoc
// @Summary      Test vector store connection with raw credentials
// @Description  Test connectivity using provided credentials without persisting. Returns detected server version.
// @Tags         VectorStore
// @Accept       json
// @Produce      json
// @Param        request  body      TestStoreRequest         true  "Engine type and connection config"
// @Success      200      {object}  map[string]interface{}   "Connection test result (success, version)"
// @Failure      400      {object}  errors.AppError          "Invalid request"
// @Failure      401      {object}  map[string]interface{}   "Unauthorized"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /system/admin/vector-stores/test [post]
func (h *VectorStoreHandler) TestStoreRaw(c *gin.Context) {
	ctx := c.Request.Context()

	var req TestStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	// Raw user input: TestRawConnection applies the engine-type allowlist,
	// required-field, and SSRF checks before any dial (unlike TestStoreByID,
	// which probes a trusted stored config).
	version, err := h.service.TestRawConnection(ctx, req.EngineType, req.ConnectionConfig)
	if err != nil {
		safeError := sanitizeStorageCheckError(err)
		logger.Warnf(ctx, "Vector store connection test failed: %s", safeError)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": safeError})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "version": version})
}
