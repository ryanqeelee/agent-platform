package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

// CreateKnowledgeBase typed-error preservation — the handler must surface
// the typed AppError (ErrVectorStoreBindingInvalid / ErrVectorStoreUnavailable)
// returned by validateVectorStoreBinding instead of stripping it into a
// generic 500 via NewInternalServerError. Without the IsAppError unwrap in
// the handler, the typed error codes would be silently nullified at the
// HTTP boundary and clients would lose the ability to branch on the cause.
//
// Shared-KB UUID suppression — responses for cross-tenant shared KBs must
// not leak the owner tenant's vector_store_id UUID. SharedStoreDisplay
// suppresses store name + engine_type for cross-tenant callers, but the
// underlying KnowledgeBase.MarshalJSON still emits the UUID; the
// buildKBResponse strip closes the gap so the UUID cannot be correlated
// across multiple shared KBs.

// stubKBCreateService drives CreateKnowledgeBase end-to-end with a
// service that returns a chosen error. Embedding the interface keeps
// any other method nil-panic'ing on purpose.
type stubKBCreateService struct {
	interfaces.KnowledgeBaseService
	createErr error
}

func (s *stubKBCreateService) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	kb.ID = "kb-new"
	kb.TenantID = 1
	return kb, nil
}

func newCreateKBRouter(svc interfaces.KnowledgeBaseService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Next()
	})
	h := &KnowledgeBaseHandler{service: svc}
	r.POST("/knowledge-bases", h.CreateKnowledgeBase)
	return r
}

func TestCreateKB_PreservesTypedErrorCode_2200(t *testing.T) {
	svc := &stubKBCreateService{
		createErr: apperrors.NewVectorStoreBindingInvalidError("vector store not found"),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/knowledge-bases",
		strings.NewReader(`{"name":"kb"}`))
	req.Header.Set("Content-Type", "application/json")
	newCreateKBRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":2200`) {
		t.Fatalf("expected envelope to contain code 2200, got %s", body)
	}
	if strings.Contains(body, `"code":1007`) || strings.Contains(body, `"code":1000`) {
		t.Fatalf("typed error must not be wrapped into a generic code, got %s", body)
	}
}

func TestCreateKB_PreservesTypedErrorCode_2201(t *testing.T) {
	svc := &stubKBCreateService{
		createErr: apperrors.NewVectorStoreUnavailableError(""),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/knowledge-bases",
		strings.NewReader(`{"name":"kb"}`))
	req.Header.Set("Content-Type", "application/json")
	newCreateKBRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":2201`) {
		t.Fatalf("expected envelope to contain code 2201, got %s", w.Body.String())
	}
}

func TestCreateKB_GenericErrorStillFallsThroughTo500(t *testing.T) {
	// A non-AppError must NOT be auto-rewritten to 200/400 — operational
	// monitoring still needs to see infrastructure failures as 5xx.
	svc := &stubKBCreateService{createErr: errSentinel("connection refused")}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/knowledge-bases",
		strings.NewReader(`{"name":"kb"}`))
	req.Header.Set("Content-Type", "application/json")
	newCreateKBRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for raw infra error, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateKB_PlanUnavailableReturnsServiceUnavailable(t *testing.T) {
	svc := &stubKBCreateService{createErr: interfaces.ErrAICapabilityUnavailable}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/knowledge-bases",
		strings.NewReader(`{"name":"kb"}`))
	req.Header.Set("Content-Type", "application/json")
	newCreateKBRouter(svc).ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// ---------------------------------------------------------------------------
// buildKBResponse must strip vector_store_id for shared KB responses
// ---------------------------------------------------------------------------

func TestBuildKBResponse_StripsVectorStoreIDForSharedKB(t *testing.T) {
	storeID := "aaaa-bbbb-cccc-dddd"
	kb := &types.KnowledgeBase{
		ID:               "kb-1",
		Name:             "shared-kb",
		TenantID:         42, // different from caller
		EmbeddingModelID: "e",
		SummaryModelID:   "s",
		VectorStoreID:    &storeID,
	}
	got := buildKBResponse(kb, types.SharedStoreDisplay(), nil)
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", got)
	}
	if _, exists := m["vector_store_id"]; exists {
		t.Fatalf("shared KB response must not expose vector_store_id, got %v", m["vector_store_id"])
	}
	if _, exists := m["vector_store_name"]; exists {
		t.Fatalf("shared KB response must not expose vector_store_name, got %v", m["vector_store_name"])
	}
	if m["vector_store_source"] != types.StoreSourceShared {
		t.Fatalf("expected vector_store_source=shared, got %v", m["vector_store_source"])
	}
	// Defensive: ensure the source UUID does not appear *anywhere* in
	// the serialized output (paranoid check against future map keys).
	serialized, _ := json.Marshal(m)
	if strings.Contains(string(serialized), storeID) {
		t.Fatalf("shared KB response leaked vector store UUID via some path: %s", serialized)
	}
}

func TestBuildKBResponse_KeepsVectorStoreIDForOwnerKB(t *testing.T) {
	// Same setup but with the user-source display — owner caller should
	// still see the UUID alongside the resolved metadata.
	storeID := "aaaa-bbbb-cccc-dddd"
	kb := &types.KnowledgeBase{
		ID:               "kb-1",
		Name:             "owner-kb",
		TenantID:         1,
		EmbeddingModelID: "e",
		SummaryModelID:   "s",
		VectorStoreID:    &storeID,
	}
	view := types.StoreDisplay{
		Name:       "prod-es",
		Source:     types.StoreSourceUser,
		EngineType: "elasticsearch",
		Status:     "available",
	}
	got := buildKBResponse(kb, view, nil)
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", got)
	}
	if m["vector_store_id"] != storeID {
		t.Fatalf("owner KB must keep vector_store_id, got %v", m["vector_store_id"])
	}
	if m["vector_store_name"] != "prod-es" {
		t.Fatalf("owner KB must surface store name, got %v", m["vector_store_name"])
	}
}

func infrastructureResponseFixture() map[string]interface{} {
	return map[string]interface{}{
		"id": "kb-1", "name": "safe-name", "knowledge_count": float64(3), "is_pinned": true,
		"my_permission": "editor", "chunking_config": map[string]interface{}{"chunk_size": float64(500)},
		"image_processing_config": map[string]interface{}{"model_id": "image-model"},
		"embedding_model_id":      "embedding-secret", "summary_model_id": "summary-secret",
		"storage_backend_id": "backend-secret", "storage_provider_config": map[string]interface{}{"secret": "storage-secret"},
		"storage_config":  map[string]interface{}{"secret_id": "legacy-storage-secret"},
		"vector_store_id": "vector-secret", "vector_store_name": "vector-name",
		"vector_store_source": "user", "vector_store_engine_type": "qdrant", "vector_store_status": "available", "vector_store_provider_token": "vector-provider-secret",
		"extract_config": map[string]interface{}{"provider": "extract-secret"}, "indexing_strategy": map[string]interface{}{"vector": true},
		"vlm_config":  map[string]interface{}{"enabled": true, "model_id": "vlm-model", "model_name": "vlm-name", "base_url": "https://vlm.invalid", "api_key": "vlm-key", "interface_type": "openai"},
		"asr_config":  map[string]interface{}{"enabled": true, "model_id": "asr-model"},
		"wiki_config": map[string]interface{}{"enabled": true, "synthesis_model_id": "wiki-model", "provider": "wiki-provider", "max_concurrency": float64(9), "ingest_map_parallel": float64(11)},
	}
}

func TestRedactKBInfrastructureAlwaysHidesPlatformFields(t *testing.T) {
	platformDetails := []string{
		"embedding-secret", "summary-secret", "backend-secret", "storage-secret", "legacy-storage-secret",
		"vector-secret", "vector-name", "qdrant", "vector-provider-secret", "extract-secret", "vlm-model", "vlm-name",
		"https://vlm.invalid", "vlm-key", "openai", "asr-model", "wiki-model", "wiki-provider",
	}
	got := redactKBInfrastructure(infrastructureResponseFixture())
	serialized, err := json.Marshal(got)
	require.NoError(t, err)
	for _, platformDetail := range platformDetails {
		require.NotContains(t, string(serialized), platformDetail)
	}
	m, ok := got.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "safe-name", m["name"])
	require.Equal(t, "editor", m["my_permission"])
	require.Equal(t, true, m["is_pinned"])
	vlm, ok := m["vlm_config"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, vlm["enabled"], "safe feature flags remain visible")
	asr, ok := m["asr_config"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, asr["enabled"])
	wiki, ok := m["wiki_config"].(map[string]interface{})
	require.True(t, ok)
	require.NotContains(t, wiki, "ingest_map_parallel", "wiki concurrency must stay hidden")
}

func TestRedactKBInfrastructureFailsClosedForUnnormalizableValue(t *testing.T) {
	got := redactKBInfrastructure(&types.KnowledgeBase{ID: "raw-kb"})
	require.Nil(t, got, "raw KB objects must never bypass the response boundary")
}

func TestSharedKBRowUsesHumanInfrastructureProjection(t *testing.T) {
	row := sharedKBRow(context.Background(), &types.SharedKnowledgeBaseInfo{
		KnowledgeBase: &types.KnowledgeBase{
			ID: "shared-kb",
			StorageConfig: types.StorageConfig{
				SecretID: "storage-id", SecretKey: "storage-secret",
			},
			VLMConfig: types.VLMConfig{APIKey: "vlm-secret", BaseURL: "https://vlm.invalid"},
		},
		SourceTenantID: 2,
	}, nil)

	body, err := json.Marshal(row)
	require.NoError(t, err)
	require.NotContains(t, string(body), "storage-secret")
	require.NotContains(t, string(body), "vlm-secret")
	require.Contains(t, string(body), "shared-kb")
}
