package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPlatformChatHistorySQLite(t *testing.T) (*gorm.DB, PlatformChatHistoryRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	for _, statement := range []string{
		`CREATE TABLE tenants (id INTEGER PRIMARY KEY, deleted_at DATETIME)`,
		`CREATE TABLE models (
			id TEXT PRIMARY KEY, tenant_id INTEGER NOT NULL, type TEXT NOT NULL,
			status TEXT NOT NULL, is_builtin INTEGER NOT NULL, deleted_at DATETIME
		)`,
		knowledgeBasesTestDDL,
		`CREATE TABLE knowledges (id TEXT PRIMARY KEY, knowledge_base_id TEXT NOT NULL, deleted_at DATETIME)`,
		`CREATE TABLE platform_chat_history_config (
			id INTEGER PRIMARY KEY CHECK (id = 1), enabled INTEGER NOT NULL,
			embedding_model_id TEXT NOT NULL, updated_by TEXT NOT NULL,
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE tenant_chat_history_indexes (
			tenant_id INTEGER PRIMARY KEY, knowledge_base_id TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		)`,
		`INSERT INTO platform_chat_history_config
			(id, enabled, embedding_model_id, updated_by, created_at, updated_at)
			VALUES (1, 1, 'embed-platform', 'migration', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	return db, NewPlatformChatHistoryRepository(db)
}

func preparedChatHistoryKB(tenantID uint64, modelID string) *types.KnowledgeBase {
	return &types.KnowledgeBase{
		ID: uuid.NewString(), Name: "__chat_history__", Type: types.KnowledgeBaseTypeDocument,
		IsTemporary: true, TenantID: tenantID, EmbeddingModelID: modelID,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestPlatformChatHistoryConfigIsSingletonAcrossTenantContexts(t *testing.T) {
	db, repo := newPlatformChatHistorySQLite(t)
	for _, tenantID := range []uint64{10001, 10004} {
		ctx := context.WithValue(t.Context(), types.TenantIDContextKey, tenantID)
		config, err := repo.GetConfig(ctx)
		require.NoError(t, err)
		require.True(t, config.Enabled)
		require.Equal(t, "embed-platform", config.EmbeddingModelID)
	}
	require.NoError(t, db.Exec("DELETE FROM platform_chat_history_config").Error)
	_, err := repo.GetConfig(t.Context())
	require.ErrorIs(t, err, ErrPlatformChatHistoryConfigMissing)
}

func TestEnsureTenantKnowledgeBaseCreatesDistinctPrivateBindings(t *testing.T) {
	db, repo := newPlatformChatHistorySQLite(t)
	require.NoError(t, db.Exec(`INSERT INTO tenants(id) VALUES (10001), (10004)`).Error)

	first, created, err := repo.EnsureTenantKnowledgeBase(t.Context(), "embed-platform", preparedChatHistoryKB(10001, "embed-platform"))
	require.NoError(t, err)
	require.True(t, created)
	second, created, err := repo.EnsureTenantKnowledgeBase(t.Context(), "embed-platform", preparedChatHistoryKB(10004, "embed-platform"))
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, uint64(10001), first.TenantID)
	require.Equal(t, uint64(10004), second.TenantID)
}

func TestEnsureTenantKnowledgeBaseRejectsDeletedTenantAndCorruptBinding(t *testing.T) {
	db, repo := newPlatformChatHistorySQLite(t)
	require.NoError(t, db.Exec(`INSERT INTO tenants(id, deleted_at) VALUES (10001, CURRENT_TIMESTAMP), (10004, NULL)`).Error)

	_, created, err := repo.EnsureTenantKnowledgeBase(t.Context(), "embed-platform", preparedChatHistoryKB(10001, "embed-platform"))
	require.Error(t, err)
	require.False(t, created)
	var deletedTenantKBs int64
	require.NoError(t, db.Table("knowledge_bases").Where("tenant_id = 10001").Count(&deletedTenantKBs).Error)
	require.Zero(t, deletedTenantKBs)

	wrong := preparedChatHistoryKB(10004, "other-model")
	require.NoError(t, db.Create(wrong).Error)
	require.NoError(t, db.Create(&types.TenantChatHistoryIndex{
		TenantID: 10004, KnowledgeBaseID: wrong.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)
	_, created, err = repo.EnsureTenantKnowledgeBase(t.Context(), "embed-platform", preparedChatHistoryKB(10004, "embed-platform"))
	require.Error(t, err)
	require.False(t, created)
}

func TestPlatformChatHistoryModelSelectionValidationAndLock(t *testing.T) {
	db, repo := newPlatformChatHistorySQLite(t)
	now := time.Now()
	for _, model := range []*types.Model{
		{ID: "embed-platform", TenantID: 0, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive, IsBuiltin: true},
		{ID: "wrong-type", TenantID: 0, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsBuiltin: true},
		{ID: "wrong-status", TenantID: 0, Type: types.ModelTypeEmbedding, Status: types.ModelStatusDownloading, IsBuiltin: true},
		{ID: "wrong-scope", TenantID: 10001, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive},
		{ID: "tenant-zero", TenantID: 0, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive},
		{ID: "replacement", TenantID: 0, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive, IsBuiltin: true},
	} {
		require.NoError(t, db.Exec(
			`INSERT INTO models(id, tenant_id, type, status, is_builtin) VALUES (?, ?, ?, ?, ?)`,
			model.ID, model.TenantID, model.Type, model.Status, model.IsBuiltin,
		).Error)
	}
	for _, modelID := range []string{"missing", "wrong-type", "wrong-status", "wrong-scope", "tenant-zero"} {
		_, err := repo.UpdateConfig(t.Context(), &types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: modelID}, "admin", now)
		require.ErrorIs(t, err, ErrPlatformChatHistoryModelInvalid)
	}

	require.NoError(t, db.Exec(`INSERT INTO tenants(id) VALUES (10001)`).Error)
	_, _, err := repo.EnsureTenantKnowledgeBase(t.Context(), "embed-platform", preparedChatHistoryKB(10001, "embed-platform"))
	require.NoError(t, err)
	_, err = repo.UpdateConfig(t.Context(), &types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: "replacement"}, "admin", now)
	require.ErrorIs(t, err, ErrPlatformChatHistoryModelLocked)
	updated, err := repo.UpdateConfig(t.Context(), &types.PlatformChatHistoryConfig{Enabled: false}, "admin", now)
	require.NoError(t, err)
	require.False(t, updated.Enabled)
	require.Empty(t, updated.EmbeddingModelID)
}
