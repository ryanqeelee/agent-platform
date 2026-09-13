package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sqlitedrv "gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newFakeESServer spins up an httptest server that responds like an
// Elasticsearch root endpoint so the connection-probe step inside
// CreateStore succeeds without needing a real ES backend.
func newFakeESServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":{"number":"7.10.1"}}`))
	}))
	t.Cleanup(srv.Close)
	// CreateStore now SSRF-validates the connection addr before the probe.
	// The httptest server listens on 127.0.0.1, which the SSRF policy blocks
	// by default, so whitelist it for the duration of the test.
	withSSRFWhitelist(t, "127.0.0.1")
	return srv
}

// ---------------------------------------------------------------------------
// Mock repository
// ---------------------------------------------------------------------------

type mockVectorStoreRepo struct {
	stores    []*types.VectorStore
	createErr error
	updateErr error
	deleteErr error
}

func (m *mockVectorStoreRepo) Create(_ context.Context, store *types.VectorStore) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.stores = append(m.stores, store)
	return nil
}

func (m *mockVectorStoreRepo) GetByID(_ context.Context, id string) (*types.VectorStore, error) {
	for _, s := range m.stores {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockVectorStoreRepo) GetDefault(_ context.Context) (*types.VectorStore, error) {
	for _, store := range m.stores {
		if store.IsDefault {
			return store, nil
		}
	}
	return nil, nil
}

func (m *mockVectorStoreRepo) List(_ context.Context) ([]*types.VectorStore, error) {
	return m.stores, nil
}

func (m *mockVectorStoreRepo) Update(_ context.Context, store *types.VectorStore) error {
	return m.updateErr
}

func (m *mockVectorStoreRepo) UpdateConnectionConfig(_ context.Context, _ *types.VectorStore) error {
	return m.updateErr
}

func (m *mockVectorStoreRepo) Delete(_ context.Context, _ string) error {
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// Mock StoreRegistry
// ---------------------------------------------------------------------------

type mockStoreRegistry struct {
	registered   map[string]bool
	unregistered []string
}

func newMockStoreRegistry() *mockStoreRegistry {
	return &mockStoreRegistry{registered: make(map[string]bool)}
}

func (m *mockStoreRegistry) RegisterWithStoreID(storeID string, _ interfaces.RetrieveEngineService) {
	m.registered[storeID] = true
}

func (m *mockStoreRegistry) GetByStoreID(storeID string) (interfaces.RetrieveEngineService, error) {
	return nil, nil
}

func (m *mockStoreRegistry) UnregisterByStoreID(storeID string) {
	m.unregistered = append(m.unregistered, storeID)
	delete(m.registered, storeID)
}

// ---------------------------------------------------------------------------
// Mock EngineFactory
// ---------------------------------------------------------------------------

func mockEngineFactory(err error) interfaces.EngineFactory {
	return func(_ context.Context, _ types.VectorStore) (interfaces.RetrieveEngineService, error) {
		if err != nil {
			return nil, err
		}
		return &mockEngineService{}, nil
	}
}

// mockEngineService satisfies interfaces.RetrieveEngineService minimally.
type mockEngineService struct{}

func (m *mockEngineService) EngineType() types.RetrieverEngineType { return "mock" }
func (m *mockEngineService) Retrieve(_ context.Context, _ types.RetrieveParams) ([]*types.RetrieveResult, error) {
	return nil, nil
}
func (m *mockEngineService) Support() []types.RetrieverType { return nil }
func (m *mockEngineService) Index(_ context.Context, _ embedding.Embedder, _ *types.IndexInfo, _ []types.RetrieverType) error {
	return nil
}
func (m *mockEngineService) BatchIndex(_ context.Context, _ embedding.Embedder, _ []*types.IndexInfo, _ []types.RetrieverType) error {
	return nil
}
func (m *mockEngineService) EstimateStorageSize(_ context.Context, _ embedding.Embedder, _ []*types.IndexInfo, _ []types.RetrieverType) int64 {
	return 0
}
func (m *mockEngineService) CopyIndices(_ context.Context, _ string, _ map[string]string, _ map[string]string, _ string, _ int, _ string) error {
	return nil
}
func (m *mockEngineService) DeleteByChunkIDList(_ context.Context, _ []string, _ int, _ string) error {
	return nil
}
func (m *mockEngineService) DeleteBySourceIDList(_ context.Context, _ []string, _ int, _ string) error {
	return nil
}
func (m *mockEngineService) DeleteByKnowledgeIDList(_ context.Context, _ []string, _ int, _ string) error {
	return nil
}
func (m *mockEngineService) BatchUpdateChunkEnabledStatus(_ context.Context, _ map[string]bool) error {
	return nil
}
func (m *mockEngineService) BatchUpdateChunkTagID(_ context.Context, _ map[string]string) error {
	return nil
}

// ---------------------------------------------------------------------------
// CreateStore tests
// ---------------------------------------------------------------------------

func TestCreateStore_Success(t *testing.T) {
	es := newFakeESServer(t)
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	store := &types.VectorStore{
		Name:       "test-es",
		EngineType: types.ElasticsearchRetrieverEngineType,
		ConnectionConfig: types.ConnectionConfig{
			Addr: es.URL,
		},
	}

	err := svc.CreateStore(context.Background(), store)
	assert.NoError(t, err)
	assert.Len(t, repo.stores, 1)
}

func TestCreateStore_ValidationError(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	tests := []struct {
		name  string
		store *types.VectorStore
	}{
		{
			name:  "empty name",
			store: &types.VectorStore{EngineType: types.PostgresRetrieverEngineType},
		},
		{
			name:  "invalid engine type",
			store: &types.VectorStore{Name: "test", EngineType: "unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.CreateStore(context.Background(), tt.store)
			require.Error(t, err)
			var appErr *errors.AppError
			assert.ErrorAs(t, err, &appErr)
		})
	}
}

func TestCreateStore_ConnectionConfigValidation(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	tests := []struct {
		name      string
		store     *types.VectorStore
		wantError bool
	}{
		{
			name: "elasticsearch without addr",
			store: &types.VectorStore{Name: "test",
				EngineType:       types.ElasticsearchRetrieverEngineType,
				ConnectionConfig: types.ConnectionConfig{},
			},
			wantError: true,
		},
		{
			name: "postgres reuses application database",
			store: &types.VectorStore{Name: "test",
				EngineType:       types.PostgresRetrieverEngineType,
				ConnectionConfig: types.ConnectionConfig{UseDefaultConnection: true},
			},
			wantError: false,
		},
		{
			name: "qdrant without host",
			store: &types.VectorStore{Name: "test",
				EngineType:       types.QdrantRetrieverEngineType,
				ConnectionConfig: types.ConnectionConfig{},
			},
			wantError: true,
		},
		{
			name: "milvus without addr",
			store: &types.VectorStore{Name: "test",
				EngineType:       types.MilvusRetrieverEngineType,
				ConnectionConfig: types.ConnectionConfig{},
			},
			wantError: true,
		},
		{
			name: "weaviate without host",
			store: &types.VectorStore{Name: "test",
				EngineType:       types.WeaviateRetrieverEngineType,
				ConnectionConfig: types.ConnectionConfig{},
			},
			wantError: true,
		},
		{
			name: "sqlite requires application database reuse",
			store: &types.VectorStore{Name: "test",
				EngineType:       types.SQLiteRetrieverEngineType,
				ConnectionConfig: types.ConnectionConfig{},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.CreateStore(context.Background(), tt.store)
			if tt.wantError {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateStore_AllowsSameEndpointAndIndex(t *testing.T) {
	es := newFakeESServer(t)
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	for _, name := range []string{"first", "second"} {
		store := &types.VectorStore{
			Name:             name,
			EngineType:       types.ElasticsearchRetrieverEngineType,
			ConnectionConfig: types.ConnectionConfig{Addr: es.URL},
			IndexConfig:      types.IndexConfig{IndexName: "shared_index"},
		}
		require.NoError(t, svc.CreateStore(context.Background(), store))
	}
	assert.Len(t, repo.stores, 2)
}

// ---------------------------------------------------------------------------
// CreateStore + Registry integration tests
// ---------------------------------------------------------------------------

func TestCreateStore_RegistersInRegistry(t *testing.T) {
	es := newFakeESServer(t)
	repo := &mockVectorStoreRepo{}
	registry := newMockStoreRegistry()
	factory := mockEngineFactory(nil)
	svc := NewVectorStoreService(repo, registry, factory, nil)

	store := &types.VectorStore{
		Name:       "test-es",
		EngineType: types.ElasticsearchRetrieverEngineType,
		ConnectionConfig: types.ConnectionConfig{
			Addr: es.URL,
		},
	}

	err := svc.CreateStore(context.Background(), store)
	require.NoError(t, err)

	// Store should be persisted AND registered in registry
	assert.Len(t, repo.stores, 1)
	assert.True(t, registry.registered[store.ID])
}

func TestCreateStore_RegistryFailureDoesNotRollBackDB(t *testing.T) {
	es := newFakeESServer(t)
	repo := &mockVectorStoreRepo{}
	registry := newMockStoreRegistry()
	factory := mockEngineFactory(assert.AnError) // factory fails
	svc := NewVectorStoreService(repo, registry, factory, nil)

	store := &types.VectorStore{
		Name:       "test-es",
		EngineType: types.ElasticsearchRetrieverEngineType,
		ConnectionConfig: types.ConnectionConfig{
			Addr: es.URL,
		},
	}

	// CreateStore should succeed even if registry fails (best-effort + self-healing)
	err := svc.CreateStore(context.Background(), store)
	assert.NoError(t, err)

	// DB should have the store
	assert.Len(t, repo.stores, 1)
	// Registry should NOT have it (factory failed)
	assert.False(t, registry.registered[store.ID])
}

func TestCreateStore_NilRegistryAndFactory(t *testing.T) {
	es := newFakeESServer(t)
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil) // no registry

	store := &types.VectorStore{
		Name:       "test-es",
		EngineType: types.ElasticsearchRetrieverEngineType,
		ConnectionConfig: types.ConnectionConfig{
			Addr: es.URL,
		},
	}

	// Should work fine without registry (degrades gracefully)
	err := svc.CreateStore(context.Background(), store)
	assert.NoError(t, err)
	assert.Len(t, repo.stores, 1)
}

// ---------------------------------------------------------------------------
// DeleteStore + Registry integration tests
//
// The DeleteStore path runs through a transactional guard that requires
// both a kbRepo and a *gorm.DB handle, so the in-memory mock cannot
// exercise it. The TestDeleteStore_Guard_* family below covers the same
// outcomes (success, missing store, registry interaction, rollback)
// against an actual sqlite transaction.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// UpdateStore tests
// ---------------------------------------------------------------------------

func TestUpdateStore_Success(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	store := &types.VectorStore{
		ID:   "test-id",
		Name: "updated-name",
	}

	err := svc.UpdateStore(context.Background(), store)
	assert.NoError(t, err)
}

func TestUpdateStore_ValidationError(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	tests := []struct {
		name  string
		store *types.VectorStore
	}{
		{
			name:  "empty name",
			store: &types.VectorStore{ID: "id", Name: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.UpdateStore(context.Background(), tt.store)
			require.Error(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// DeleteStore tests
// ---------------------------------------------------------------------------

// TestDeleteStore_Success and TestDeleteStore_RepoError have been replaced
// by the TestDeleteStore_Guard_* family below, which exercises the
// transactional delete-guard path against an actual sqlite database (a mock
// repo would not honor the row-lock + binding-count semantics).

// ---------------------------------------------------------------------------
// SaveDetectedVersion tests
// ---------------------------------------------------------------------------

func TestSaveDetectedVersion_Success(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	store := &types.VectorStore{
		ID:               "store-1",
		ConnectionConfig: types.ConnectionConfig{Addr: "http://es:9200"},
	}

	err := svc.SaveDetectedVersion(context.Background(), store, "7.10.1")
	assert.NoError(t, err)
}

func TestSaveDetectedVersion_RepoError(t *testing.T) {
	repo := &mockVectorStoreRepo{updateErr: assert.AnError}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	store := &types.VectorStore{ID: "store-1"}
	err := svc.SaveDetectedVersion(context.Background(), store, "8.11.0")
	assert.Error(t, err)
}

func TestSaveDetectedVersion_DoesNotMutateOriginal(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	store := &types.VectorStore{
		ID:               "store-1",
		ConnectionConfig: types.ConnectionConfig{Version: "old"},
	}

	err := svc.SaveDetectedVersion(context.Background(), store, "new")
	require.NoError(t, err)

	// Original store must not be mutated
	assert.Equal(t, "old", store.ConnectionConfig.Version)
}

// ---------------------------------------------------------------------------
// TestConnection tests
// ---------------------------------------------------------------------------

func TestTestConnection_UnsupportedEngineType(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	_, err := svc.TestConnection(context.Background(), "unknown_engine", types.ConnectionConfig{})
	require.Error(t, err)

	var appErr *errors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, errors.ErrBadRequest, appErr.Code)
}

func TestTestConnection_SQLiteAlwaysSucceeds(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	version, err := svc.TestConnection(context.Background(), types.SQLiteRetrieverEngineType, types.ConnectionConfig{})
	assert.NoError(t, err)
	assert.Empty(t, version)
}

func TestTestConnection_PostgresDefaultConnection(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	version, err := svc.TestConnection(context.Background(), types.PostgresRetrieverEngineType,
		types.ConnectionConfig{UseDefaultConnection: true})
	assert.NoError(t, err)
	assert.Empty(t, version) // default connection cannot detect version without DB handle
}

func TestTestConnection_DorisInvalidAddr(t *testing.T) {
	// 给一个不可达的地址 + 5s timeout，期望返回 BadRequestError 而非 panic。
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err := svc.TestConnection(ctx, types.DorisRetrieverEngineType, types.ConnectionConfig{
		Addr:     "127.0.0.1:1", // 一定不可连通
		Database: "weknora",
		Username: "root",
	})
	require.Error(t, err)
}

func TestTestConnection_DorisMissingAddr(t *testing.T) {
	repo := &mockVectorStoreRepo{}
	svc := NewVectorStoreService(repo, nil, nil, nil)

	_, err := svc.TestConnection(context.Background(), types.DorisRetrieverEngineType,
		types.ConnectionConfig{})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// validateConnectionConfig tests
// ---------------------------------------------------------------------------

func TestValidateConnectionConfig(t *testing.T) {
	tests := []struct {
		name       string
		engineType types.RetrieverEngineType
		config     types.ConnectionConfig
		wantError  bool
	}{
		{
			name:       "elasticsearch valid",
			engineType: types.ElasticsearchRetrieverEngineType,
			config:     types.ConnectionConfig{Addr: "http://es:9200"},
			wantError:  false,
		},
		{
			name:       "elasticsearch missing addr",
			engineType: types.ElasticsearchRetrieverEngineType,
			config:     types.ConnectionConfig{},
			wantError:  true,
		},
		{
			name:       "postgres with default connection",
			engineType: types.PostgresRetrieverEngineType,
			config:     types.ConnectionConfig{UseDefaultConnection: true},
			wantError:  false,
		},
		{
			name:       "postgres with external addr",
			engineType: types.PostgresRetrieverEngineType,
			config:     types.ConnectionConfig{Addr: "postgres://host:5432/db"},
			wantError:  true,
		},
		{
			name:       "postgres without addr or default",
			engineType: types.PostgresRetrieverEngineType,
			config:     types.ConnectionConfig{},
			wantError:  true,
		},
		{
			name:       "qdrant valid",
			engineType: types.QdrantRetrieverEngineType,
			config:     types.ConnectionConfig{Host: "qdrant-host"},
			wantError:  false,
		},
		{
			name:       "qdrant missing host",
			engineType: types.QdrantRetrieverEngineType,
			config:     types.ConnectionConfig{},
			wantError:  true,
		},
		{
			name:       "milvus valid",
			engineType: types.MilvusRetrieverEngineType,
			config:     types.ConnectionConfig{Addr: "milvus:19530"},
			wantError:  false,
		},
		{
			name:       "milvus missing addr",
			engineType: types.MilvusRetrieverEngineType,
			config:     types.ConnectionConfig{},
			wantError:  true,
		},
		{
			name:       "weaviate valid",
			engineType: types.WeaviateRetrieverEngineType,
			config:     types.ConnectionConfig{Host: "weaviate:8080"},
			wantError:  false,
		},
		{
			name:       "weaviate missing host",
			engineType: types.WeaviateRetrieverEngineType,
			config:     types.ConnectionConfig{},
			wantError:  true,
		},
		{
			name:       "sqlite with default connection",
			engineType: types.SQLiteRetrieverEngineType,
			config:     types.ConnectionConfig{UseDefaultConnection: true},
			wantError:  false,
		},
		{
			name:       "sqlite without default connection",
			engineType: types.SQLiteRetrieverEngineType,
			config:     types.ConnectionConfig{},
			wantError:  true,
		},
		{
			name:       "doris valid",
			engineType: types.DorisRetrieverEngineType,
			config:     types.ConnectionConfig{Addr: "doris-fe:9030", Database: "weknora"},
			wantError:  false,
		},
		{
			name:       "doris missing addr",
			engineType: types.DorisRetrieverEngineType,
			config:     types.ConnectionConfig{Database: "weknora"},
			wantError:  true,
		},
		{
			name:       "doris missing database",
			engineType: types.DorisRetrieverEngineType,
			config:     types.ConnectionConfig{Addr: "doris-fe:9030"},
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectionConfig(tt.engineType, tt.config)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DeleteStore guard + ResolveStoreView + BatchResolveStoreView integration
//
// These use a real sqlite in-memory database because the delete guard relies
// on GORM transactions and the soft-delete auto-scope; an interface-level
// mock would not catch divergence between the row-lock path and the count.
// ---------------------------------------------------------------------------

// guardTestDDL inlines the subset of migrations/sqlite/000000_init.up.sql
// that the delete-guard tests touch. We do not use AutoMigrate because the
// KnowledgeBase struct carries `type:jsonb` GORM tags that SQLite cannot
// map cleanly.
const guardTestDDL = `
CREATE TABLE IF NOT EXISTS vector_stores (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    engine_type VARCHAR(50) NOT NULL,
    connection_config TEXT NOT NULL DEFAULT '{}',
    index_config TEXT NOT NULL DEFAULT '{}',
	is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL
);
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    tenant_id INTEGER NOT NULL,
	ai_capability_plan_version_id VARCHAR(128),
    creator_id VARCHAR(36),
    type VARCHAR(32) NOT NULL DEFAULT 'document',
    chunking_config TEXT NOT NULL DEFAULT '{}',
    image_processing_config TEXT NOT NULL DEFAULT '{}',
    embedding_model_id VARCHAR(64) NOT NULL,
    summary_model_id VARCHAR(64) NOT NULL,
    cos_config TEXT NOT NULL DEFAULT '{}',
    storage_provider_config TEXT DEFAULT NULL,
    vlm_config TEXT NOT NULL DEFAULT '{}',
    extract_config TEXT NULL DEFAULT NULL,
    faq_config TEXT,
    question_generation_config TEXT NULL,
    auto_tag_config TEXT NULL,
    is_temporary BOOLEAN NOT NULL DEFAULT 0,
    is_pinned INTEGER NOT NULL DEFAULT 0,
    pinned_at DATETIME NULL,
    asr_config TEXT,
		vector_store_id VARCHAR(36),
		storage_backend_id VARCHAR(36),
    wiki_config TEXT,
    indexing_strategy TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);
`

func newGuardTestService(t *testing.T) (*vectorStoreService, *gorm.DB, *mockStoreRegistry) {
	t.Helper()
	db, err := gorm.Open(sqlitedrv.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(guardTestDDL).Error)

	registry := newMockStoreRegistry()
	svc := &vectorStoreService{
		repo:          &realStoreRepo{db: db},
		storeRegistry: registry,
		factory:       nil,
		db:            db,
	}
	return svc, db, registry
}

// realStoreRepo is a hand-rolled VectorStoreRepository against an in-memory
// sqlite handle. We avoid pulling in the repository package to keep this
// test file self-contained.
type realStoreRepo struct{ db *gorm.DB }

func (r *realStoreRepo) Create(ctx context.Context, s *types.VectorStore) error {
	return r.db.WithContext(ctx).Create(s).Error
}
func (r *realStoreRepo) GetByID(ctx context.Context, id string) (*types.VectorStore, error) {
	var s types.VectorStore
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
func (r *realStoreRepo) GetDefault(ctx context.Context) (*types.VectorStore, error) {
	var store types.VectorStore
	err := r.db.WithContext(ctx).Where("is_default = ?", true).First(&store).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &store, err
}
func (r *realStoreRepo) List(ctx context.Context) ([]*types.VectorStore, error) {
	var stores []*types.VectorStore
	err := r.db.WithContext(ctx).Find(&stores).Error
	return stores, err
}
func (r *realStoreRepo) Update(_ context.Context, _ *types.VectorStore) error {
	return nil
}
func (r *realStoreRepo) UpdateConnectionConfig(_ context.Context, _ *types.VectorStore) error {
	return nil
}
func (r *realStoreRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.VectorStore{}).Error
}

func insertGuardStore(t *testing.T, db *gorm.DB, id string, tenantID uint64) {
	t.Helper()
	_ = tenantID
	require.NoError(t, db.Create(&types.VectorStore{
		ID: id, Name: id, EngineType: types.QdrantRetrieverEngineType,
	}).Error)
}

func insertGuardKB(t *testing.T, db *gorm.DB, tenantID uint64, vsid *string) string {
	t.Helper()
	id := "kb-" + tenantID2s(tenantID) + "-" + ptrOrEmpty(vsid)
	kb := &types.KnowledgeBase{
		ID:               id,
		Name:             id,
		TenantID:         tenantID,
		EmbeddingModelID: "embed",
		SummaryModelID:   "sum",
		VectorStoreID:    vsid,
	}
	require.NoError(t, db.Create(kb).Error)
	return id
}

func tenantID2s(t uint64) string {
	if t == 1 {
		return "t1"
	}
	if t == 2 {
		return "t2"
	}
	return "tN"
}
func ptrOrEmpty(p *string) string {
	if p == nil {
		return "nil"
	}
	return *p
}

func TestDeleteStore_Guard_Success(t *testing.T) {
	ctx := context.Background()
	svc, db, registry := newGuardTestService(t)
	insertGuardStore(t, db, "store-A", 1)

	err := svc.DeleteStore(ctx, "store-A")
	require.NoError(t, err)

	// store row soft-deleted, no longer visible to default GORM scope.
	var got types.VectorStore
	err = db.Where("id = ?", "store-A").First(&got).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// registry unregister was invoked once.
	require.Equal(t, []string{"store-A"}, registry.unregistered)
}

func TestDeleteStore_Guard_RejectsBoundKB(t *testing.T) {
	ctx := context.Background()
	svc, db, registry := newGuardTestService(t)
	insertGuardStore(t, db, "store-A", 1)
	storeID := "store-A"
	insertGuardKB(t, db, 1, &storeID)

	err := svc.DeleteStore(ctx, "store-A")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still has 1 knowledge base")

	// store row must still be present (rollback succeeded).
	var got types.VectorStore
	require.NoError(t, db.Where("id = ?", "store-A").First(&got).Error)
	require.Empty(t, registry.unregistered, "registry must NOT be unregistered when guard fires")
}

func TestDeleteStore_Guard_IgnoresSoftDeletedKB(t *testing.T) {
	ctx := context.Background()
	svc, db, registry := newGuardTestService(t)
	insertGuardStore(t, db, "store-A", 1)
	storeID := "store-A"
	kbID := insertGuardKB(t, db, 1, &storeID)
	require.NoError(t, db.Where("id = ?", kbID).Delete(&types.KnowledgeBase{}).Error)

	err := svc.DeleteStore(ctx, "store-A")
	require.NoError(t, err)
	require.Equal(t, []string{"store-A"}, registry.unregistered)
}

func TestDeleteStore_Guard_RejectsBoundKBAcrossTenants(t *testing.T) {
	ctx := context.Background()
	svc, db, registry := newGuardTestService(t)
	insertGuardStore(t, db, "store-A", 1)
	storeID := "store-A"
	insertGuardKB(t, db, 2, &storeID)

	err := svc.DeleteStore(ctx, "store-A")
	require.ErrorContains(t, err, "still has 1 knowledge base")
	require.Empty(t, registry.unregistered)
}

func TestDeleteStore_Guard_StoreNotFound(t *testing.T) {
	ctx := context.Background()
	svc, _, registry := newGuardTestService(t)

	err := svc.DeleteStore(ctx, "store-missing")
	require.Error(t, err)
	appErr, ok := errors.IsAppError(err)
	require.True(t, ok)
	assert.Equal(t, 404, appErr.HTTPCode)
	require.Empty(t, registry.unregistered)
}

func TestDeleteStore_Guard_RejectsCount3(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newGuardTestService(t)
	insertGuardStore(t, db, "store-A", 1)
	storeID := "store-A"
	for i := 0; i < 3; i++ {
		kb := &types.KnowledgeBase{
			ID:   "kb-multi-" + tenantID2s(1) + "-" + ptrOrEmpty(&storeID) + "-" + string(rune('a'+i)),
			Name: "kb", TenantID: 1, EmbeddingModelID: "e", SummaryModelID: "s",
			VectorStoreID: &storeID,
		}
		require.NoError(t, db.Create(kb).Error)
	}

	err := svc.DeleteStore(ctx, "store-A")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still has 3 knowledge base")
}

func TestUnregisterSafely_PanicRecovered(t *testing.T) {
	svc := &vectorStoreService{
		storeRegistry: panickingRegistry{},
	}
	// Must not panic out of the test process.
	svc.unregisterSafely(context.Background(), "store-A")
}

type panickingRegistry struct{}

func (panickingRegistry) RegisterWithStoreID(string, interfaces.RetrieveEngineService) {}
func (panickingRegistry) GetByStoreID(string) (interfaces.RetrieveEngineService, error) {
	return nil, nil
}
func (panickingRegistry) UnregisterByStoreID(string) { panic("registry exploded") }

// ---------------------------------------------------------------------------
// ResolveStoreView / BatchResolveStoreView
// ---------------------------------------------------------------------------

func TestResolveStoreView(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newGuardTestService(t)
	insertGuardStore(t, db, "store-A", 1)
	require.NoError(t, db.Model(&types.VectorStore{}).Where("id = ?", "store-A").Update("is_default", true).Error)

	t.Run("empty store id returns global default", func(t *testing.T) {
		v, err := svc.ResolveStoreView(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, "store-A", v.Name)
		assert.Equal(t, types.StoreSourceUser, v.Source)
	})
	t.Run("DB hit", func(t *testing.T) {
		v, err := svc.ResolveStoreView(ctx, "store-A")
		require.NoError(t, err)
		assert.Equal(t, "store-A", v.Name)
		assert.Equal(t, types.StoreSourceUser, v.Source)
		assert.Equal(t, string(types.QdrantRetrieverEngineType), v.EngineType)
	})
	t.Run("miss returns unavailable", func(t *testing.T) {
		v, err := svc.ResolveStoreView(ctx, "store-zzz")
		require.NoError(t, err)
		assert.Equal(t, types.StoreSourceUnavailable, v.Source)
	})
	t.Run("global store is visible independent of tenant ownership", func(t *testing.T) {
		insertGuardStore(t, db, "store-B", 2)
		v, err := svc.ResolveStoreView(ctx, "store-B")
		require.NoError(t, err)
		assert.Equal(t, types.StoreSourceUser, v.Source)
	})
}

func TestBatchResolveStoreView(t *testing.T) {
	ctx := context.Background()
	svc, db, _ := newGuardTestService(t)
	insertGuardStore(t, db, "store-A", 1)
	insertGuardStore(t, db, "store-B", 1)
	require.NoError(t, db.Model(&types.VectorStore{}).Where("id = ?", "store-A").Update("is_default", true).Error)

	got, err := svc.BatchResolveStoreView(ctx, []string{
		"store-A", "store-zzz", "__env_qd__", "",
	})
	require.NoError(t, err)
	require.Len(t, got, 4)
	assert.Equal(t, types.StoreSourceUser, got["store-A"].Source)
	assert.Equal(t, "store-A", got["store-A"].Name)
	assert.Equal(t, types.StoreSourceUnavailable, got["store-zzz"].Source)
	assert.Equal(t, types.StoreSourceUnavailable, got["__env_qd__"].Source)
	assert.Equal(t, types.StoreSourceUser, got[""].Source)
	assert.Equal(t, "store-A", got[""].Name)
}

func TestBatchResolveStoreView_Empty(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newGuardTestService(t)
	got, err := svc.BatchResolveStoreView(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
