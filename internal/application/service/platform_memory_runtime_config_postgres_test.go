package service

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newPlatformMemoryRuntimePostgres(t *testing.T) (*gorm.DB, *PlatformMemoryRuntimeConfigService, *types.Model) {
	t.Helper()
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	schema := "weknora_platform_memory_runtime_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}, &types.PlatformMemoryRuntimeConfig{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE platform_chat_history_config (
			id SMALLINT PRIMARY KEY CHECK (id = 1), enabled BOOLEAN NOT NULL,
			embedding_model_id VARCHAR(64) NOT NULL
		);
		CREATE TABLE knowledge_bases (id VARCHAR(36) PRIMARY KEY, embedding_model_id VARCHAR(64) NOT NULL);
		CREATE TABLE tenant_chat_history_indexes (
			tenant_id BIGINT PRIMARY KEY, knowledge_base_id VARCHAR(36) NOT NULL UNIQUE REFERENCES knowledge_bases(id)
		);
		INSERT INTO platform_chat_history_config(id, enabled, embedding_model_id) VALUES (1, false, '');
	`).Error)
	model := &types.Model{
		ID: "memory-embedding", TenantID: 0, Name: "embedding-a",
		Type: types.ModelTypeEmbedding, Source: types.ModelSourceRemote,
		IsBuiltin: true, Status: types.ModelStatusActive,
		Parameters: types.ModelParameters{EmbeddingParameters: types.EmbeddingParameters{Dimension: 768}},
	}
	require.NoError(t, db.Create(model).Error)
	require.NoError(t, db.Create(&types.PlatformMemoryRuntimeConfig{
		ID: types.PlatformMemoryRuntimeConfigSingletonID, Runtime: types.DefaultMemoryRuntimeConfig(),
		UpdatedBy: "migration",
	}).Error)
	return db, NewPlatformMemoryRuntimeConfigService(repository.NewPlatformMemoryRuntimeConfigRepository(db)), model
}

func TestPostgresPlatformMemoryAssignmentSerializesSemanticModelUpdate(t *testing.T) {
	db, service, model := newPlatformMemoryRuntimePostgres(t)
	blocker := db.Begin()
	require.NoError(t, blocker.Error)
	defer blocker.Rollback()
	require.NoError(t, blocker.Exec(
		"UPDATE platform_memory_runtime_config SET updated_by = 'blocker' WHERE id = 1",
	).Error)

	runtime := types.DefaultMemoryRuntimeConfig()
	runtime.EmbeddingModelID = model.ID
	assignment := make(chan error, 1)
	go func() {
		ctx := context.WithValue(t.Context(), types.UserIDContextKey, "platform-admin")
		ctx = context.WithValue(ctx, types.SystemAdminContextKey, true)
		_, err := service.Update(
			ctx, runtime,
		)
		assignment <- err
	}()
	select {
	case err := <-assignment:
		t.Fatalf("platform memory assignment did not wait for singleton lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	incoming := *model
	incoming.Name = "embedding-b"
	mutation := make(chan error, 1)
	go func() { mutation <- repository.NewModelRepository(db).Update(t.Context(), &incoming) }()
	select {
	case err := <-mutation:
		t.Fatalf("model mutation did not wait for memory assignment model lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, blocker.Commit().Error)
	require.NoError(t, <-assignment)
	select {
	case err := <-mutation:
		require.ErrorIs(t, err, types.ErrModelPlatformRuntimeBinding)
	case <-time.After(5 * time.Second):
		t.Fatal("model mutation did not finish after memory assignment committed")
	}
}
