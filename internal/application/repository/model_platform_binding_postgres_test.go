package repository

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newPlatformModelBindingPostgres(t *testing.T) (*gorm.DB, *types.Model) {
	t.Helper()
	db := newIsolatedPostgresTestDatabase(t, "weknora_platform_model_binding_")
	require.NoError(t, db.AutoMigrate(&types.Model{}, &types.PlatformMemoryRuntimeConfig{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE platform_chat_history_config (
			id SMALLINT PRIMARY KEY CHECK (id = 1), enabled BOOLEAN NOT NULL,
			embedding_model_id VARCHAR(64) NOT NULL, updated_by VARCHAR(36) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE knowledge_bases (id VARCHAR(36) PRIMARY KEY, embedding_model_id VARCHAR(64) NOT NULL);
		CREATE TABLE tenant_chat_history_indexes (
			tenant_id BIGINT PRIMARY KEY, knowledge_base_id VARCHAR(36) NOT NULL UNIQUE REFERENCES knowledge_bases(id)
		);
		INSERT INTO platform_chat_history_config(id, enabled, embedding_model_id, updated_by) VALUES (1, false, '', 'migration');
	`).Error)
	model := &types.Model{
		ID: "bound-embedding", TenantID: 0, Name: "embedding-a",
		Type: types.ModelTypeEmbedding, Source: types.ModelSourceRemote,
		IsBuiltin: true, Status: types.ModelStatusActive,
		Parameters: types.ModelParameters{EmbeddingParameters: types.EmbeddingParameters{Dimension: 768}},
	}
	require.NoError(t, db.Create(model).Error)
	require.NoError(t, db.Create(&types.PlatformMemoryRuntimeConfig{
		ID: types.PlatformMemoryRuntimeConfigSingletonID, Runtime: types.DefaultMemoryRuntimeConfig(),
		UpdatedBy: "migration",
	}).Error)
	return db, model
}

func waitForBlockedModelMutation(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("model mutation did not wait for assignment lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
}

func requireRejectedModelMutation(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		require.ErrorIs(t, err, types.ErrModelPlatformRuntimeBinding)
	case <-time.After(5 * time.Second):
		t.Fatal("model mutation did not finish after assignment committed")
	}
}

func TestPostgresChatHistoryAssignmentSerializesSemanticModelUpdate(t *testing.T) {
	db, model := newPlatformModelBindingPostgres(t)
	blocker := db.Begin()
	require.NoError(t, blocker.Error)
	defer blocker.Rollback()
	require.NoError(t, blocker.Exec(
		"UPDATE platform_chat_history_config SET updated_by = 'blocker' WHERE id = 1",
	).Error)
	assignment := make(chan error, 1)
	go func() {
		_, err := NewPlatformChatHistoryRepository(db).UpdateConfig(
			t.Context(), &types.PlatformChatHistoryConfig{
				Enabled: true, EmbeddingModelID: model.ID,
			}, "platform-admin", time.Now(),
		)
		assignment <- err
	}()
	select {
	case err := <-assignment:
		t.Fatalf("chat-history assignment did not wait for singleton lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	incoming := *model
	incoming.Parameters.EmbeddingParameters.Dimension = 1536
	mutation := make(chan error, 1)
	go func() { mutation <- NewModelRepository(db).Update(t.Context(), &incoming) }()
	waitForBlockedModelMutation(t, mutation)
	require.NoError(t, blocker.Commit().Error)
	require.NoError(t, <-assignment)
	requireRejectedModelMutation(t, mutation)
}
