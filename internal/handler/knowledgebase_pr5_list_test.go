package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ListKnowledgeBases resolves store views in batch, but human responses never
// expose vector_store_* infrastructure. The list path still avoids an N+1
// lookup; explicit technical API-key authority is tested at the shared
// redaction boundary.

// stubListKBService returns a fixed slice from ListKnowledgeBases. Only
// the methods exercised by ListKnowledgeBases are implemented; embedding
// the interface keeps the rest nil-panic'ing intentionally.
type stubListKBService struct {
	interfaces.KnowledgeBaseService
	kbs []*types.KnowledgeBase
}

func (s *stubListKBService) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

type stubMoveTargetKBService struct {
	interfaces.KnowledgeBaseService
	source *types.KnowledgeBase
	kbs    []*types.KnowledgeBase
}

func (s *stubMoveTargetKBService) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if s.source != nil && s.source.ID == id {
		return s.source, nil
	}
	return nil, errSentinel("knowledge base not found")
}

func (s *stubMoveTargetKBService) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

func newMoveTargetsRouter(t *testing.T, svc interfaces.KnowledgeBaseService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleAdmin)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &KnowledgeBaseHandler{service: svc}
	r.GET("/knowledge-bases/:id/move-targets", h.ListMoveTargets)
	return r
}

// stubVectorStoreService satisfies the two service methods the list
// path depends on: BatchResolveStoreView for bound KBs and
// EnvDefaultStoreView for env-fallback KBs. ResolveStoreView is
// intentionally left nil because ListKnowledgeBases must never reach
// into the single-KB resolver — doing so per row would be the N+1
// pattern this path is designed to avoid.
type stubVectorStoreService struct {
	interfaces.VectorStoreService
	batch      map[string]types.StoreDisplay
	batchCalls int
	batchErr   error
	envView    types.StoreDisplay
}

func (s *stubVectorStoreService) BatchResolveStoreView(
	_ context.Context, _ uint64, storeIDs []string,
) (map[string]types.StoreDisplay, error) {
	s.batchCalls++
	if s.batchErr != nil {
		return nil, s.batchErr
	}
	out := make(map[string]types.StoreDisplay, len(storeIDs))
	for _, id := range storeIDs {
		if v, ok := s.batch[id]; ok {
			out[id] = v
		} else {
			out[id] = types.UnavailableStoreDisplay()
		}
	}
	return out, nil
}

func (s *stubVectorStoreService) EnvDefaultStoreView(_ context.Context) types.StoreDisplay {
	if s.envView.Source == "" {
		return types.DefaultStoreDisplay()
	}
	return s.envView
}

func newListKBRouter(
	t *testing.T,
	svc interfaces.KnowledgeBaseService,
	vss interfaces.VectorStoreService,
) *gin.Engine {
	return newListKBRouterForRole(t, svc, vss, types.TenantRoleAdmin)
}

func newListKBRouterForRole(
	t *testing.T,
	svc interfaces.KnowledgeBaseService,
	vss interfaces.VectorStoreService,
	role types.TenantRole,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "u-test")
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, role)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &KnowledgeBaseHandler{service: svc, vectorStoreService: vss}
	r.GET("/knowledge-bases", h.ListKnowledgeBases)
	return r
}

func TestListKB_RedactsInfrastructureForAllHumanRoles(t *testing.T) {
	storeID := "store-secret"
	kb := &types.KnowledgeBase{
		ID: "kb", TenantID: 1, EmbeddingModelID: "embed-secret",
		SummaryModelID: "summary-secret", StorageBackendID: &storeID, VectorStoreID: &storeID,
	}
	vss := &stubVectorStoreService{batch: map[string]types.StoreDisplay{
		storeID: {Name: "private-store", Source: types.StoreSourceUser, EngineType: "qdrant", Status: "available"},
	}}
	for _, role := range []types.TenantRole{types.TenantRoleViewer, types.TenantRoleContributor, types.TenantRoleAdmin, types.TenantRoleOwner} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/knowledge-bases", nil)
		newListKBRouterForRole(t, &stubListKBService{kbs: []*types.KnowledgeBase{kb}}, vss, role).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("role %s: status=%d body=%s", role, w.Code, w.Body.String())
		}
		for _, platformDetail := range []string{"embed-secret", "summary-secret", "store-secret", "private-store", "qdrant"} {
			if strings.Contains(w.Body.String(), platformDetail) {
				t.Fatalf("role %s: response leaked %q: %s", role, platformDetail, w.Body.String())
			}
		}
	}
}

func TestListKB_HidesInfrastructureForHumanAdminAcrossStoreKinds(t *testing.T) {
	storeUserA := "aaaa-bbbb-cccc-dddd"
	storeForeign := "ffff-eeee-dddd-cccc"

	kbs := []*types.KnowledgeBase{
		{ID: "kb-env", Name: "env", TenantID: 1},
		{ID: "kb-bound", Name: "bound", TenantID: 1, VectorStoreID: &storeUserA},
		{ID: "kb-shared", Name: "shared", TenantID: 99, VectorStoreID: &storeForeign},
	}
	vss := &stubVectorStoreService{
		batch: map[string]types.StoreDisplay{
			storeUserA: {
				Name:       "prod-qdrant",
				Source:     types.StoreSourceUser,
				EngineType: "qdrant",
				Status:     "available",
			},
			// storeForeign is intentionally absent — shared KBs do not
			// flow through BatchResolveStoreView so the stub must never
			// see it. The assertion below confirms.
		},
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases", nil)
	newListKBRouter(t, &stubListKBService{kbs: kbs}, vss).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var envelope struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if !envelope.Success || len(envelope.Data) != 3 {
		t.Fatalf("expected 3 rows, got %d body=%s", len(envelope.Data), w.Body.String())
	}

	byID := map[string]map[string]interface{}{}
	for _, row := range envelope.Data {
		byID[row["id"].(string)] = row
	}

	for id, row := range byID {
		for key := range row {
			if strings.HasPrefix(key, "vector_store_") {
				t.Fatalf("human admin row %s leaked %s: %v", id, key, row[key])
			}
		}
	}
	for _, platformDetail := range []string{storeUserA, storeForeign, "prod-qdrant", "qdrant"} {
		if strings.Contains(w.Body.String(), platformDetail) {
			t.Fatalf("human admin list leaked %q: %s", platformDetail, w.Body.String())
		}
	}
}

func TestListKB_BatchesStoreLookupsToAvoidNPlus1(t *testing.T) {
	// Five KBs bound to three distinct stores. The list endpoint must
	// resolve them in a single BatchResolveStoreView call regardless of
	// row count — calling the per-KB ResolveStoreView path inside the
	// loop would issue one service call per KB (the N+1 pattern this
	// test pins against).
	s1, s2, s3 := "store-1", "store-2", "store-3"
	kbs := []*types.KnowledgeBase{
		{ID: "a", TenantID: 1, VectorStoreID: &s1},
		{ID: "b", TenantID: 1, VectorStoreID: &s2},
		{ID: "c", TenantID: 1, VectorStoreID: &s1}, // dup
		{ID: "d", TenantID: 1, VectorStoreID: &s3},
		{ID: "e", TenantID: 1}, // env, no store call
	}
	vss := &stubVectorStoreService{
		batch: map[string]types.StoreDisplay{
			s1: {Name: "s1", Source: types.StoreSourceUser, EngineType: "qdrant", Status: "available"},
			s2: {Name: "s2", Source: types.StoreSourceUser, EngineType: "postgres", Status: "available"},
			s3: {Name: "s3", Source: types.StoreSourceUser, EngineType: "weaviate", Status: "available"},
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases", nil)
	newListKBRouter(t, &stubListKBService{kbs: kbs}, vss).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if vss.batchCalls != 1 {
		t.Fatalf("expected exactly 1 batch store-view call (N+1 protection), got %d", vss.batchCalls)
	}
}

func TestListKB_GracefullyDegradesWhenBatchResolveFails(t *testing.T) {
	// If the store-view resolver fails, the list response must still
	// succeed — bound KBs render as unavailable. The list endpoint is
	// not allowed to 500 just because the vector-store service is
	// momentarily unhealthy.
	storeID := "aaaa-bbbb"
	kbs := []*types.KnowledgeBase{
		{ID: "kb", TenantID: 1, VectorStoreID: &storeID},
	}
	vss := &stubVectorStoreService{batchErr: errSentinel("infra glitch")}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases", nil)
	newListKBRouter(t, &stubListKBService{kbs: kbs}, vss).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even when batch resolve fails, got %d body=%s", w.Code, w.Body.String())
	}
	var envelope struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &envelope)
	if len(envelope.Data) != 1 {
		t.Fatalf("expected 1 row, got %d", len(envelope.Data))
	}
	for key := range envelope.Data[0] {
		if strings.HasPrefix(key, "vector_store_") {
			t.Errorf("human response must not expose fallback infrastructure %s", key)
		}
	}
}

func TestListMoveTargets_RedactsInfrastructureForHumanAdmin(t *testing.T) {
	storageID := "move-target-storage-secret"
	source := &types.KnowledgeBase{ID: "source", TenantID: 1, Type: "document", EmbeddingModelID: "embedding-secret"}
	target := &types.KnowledgeBase{
		ID: "target", TenantID: 1, Type: "document", EmbeddingModelID: "embedding-secret",
		SummaryModelID: "summary-secret", StorageBackendID: &storageID, VectorStoreID: &storageID,
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/source/move-targets", nil)
	newMoveTargetsRouter(t, &stubMoveTargetKBService{source: source, kbs: []*types.KnowledgeBase{source, target}}).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	for _, platformDetail := range []string{"embedding-secret", "summary-secret", "move-target-storage-secret"} {
		if strings.Contains(w.Body.String(), platformDetail) {
			t.Fatalf("human move-target response leaked %q: %s", platformDetail, w.Body.String())
		}
	}
}
