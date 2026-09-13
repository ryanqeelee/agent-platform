package repository

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newPlatformChatHistoryPostgres(t *testing.T) (*gorm.DB, PlatformChatHistoryRepository) {
	t.Helper()
	db := newIsolatedPostgresTestDatabase(t, "weknora_platform_chat_history_")
	for _, statement := range []string{
		`CREATE TABLE tenants (
			id INTEGER PRIMARY KEY, chat_history_config JSONB, deleted_at TIMESTAMPTZ
		)`,
		`CREATE TABLE models (
			id VARCHAR(64) PRIMARY KEY, tenant_id INTEGER NOT NULL, type VARCHAR(50) NOT NULL,
			status VARCHAR(50) NOT NULL, is_builtin BOOLEAN NOT NULL DEFAULT FALSE, deleted_at TIMESTAMPTZ
		)`,
		`CREATE TABLE knowledge_bases (
			id VARCHAR(36) PRIMARY KEY, name VARCHAR(255) NOT NULL, description TEXT,
			tenant_id INTEGER NOT NULL, type VARCHAR(32) NOT NULL DEFAULT 'document',
			is_temporary BOOLEAN NOT NULL DEFAULT FALSE, creator_id VARCHAR(36),
			ai_capability_plan_version_id VARCHAR(128),
			chunking_config JSONB NOT NULL DEFAULT '{}', image_processing_config JSONB NOT NULL DEFAULT '{}',
			embedding_model_id VARCHAR(64) NOT NULL, summary_model_id VARCHAR(64) NOT NULL DEFAULT '',
			cos_config JSONB NOT NULL DEFAULT '{}', vlm_config JSONB NOT NULL DEFAULT '{}',
			asr_config JSONB DEFAULT '{}', storage_provider_config JSONB,
			storage_backend_id VARCHAR(36), vector_store_id VARCHAR(36), extract_config JSONB,
			faq_config JSONB, question_generation_config JSONB, auto_tag_config JSONB,
			wiki_config JSONB, indexing_strategy JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ, deleted_at TIMESTAMPTZ
		)`,
		`CREATE TABLE knowledges (id VARCHAR(36) PRIMARY KEY, knowledge_base_id VARCHAR(36) NOT NULL, deleted_at TIMESTAMPTZ)`,
		migrationSQL(t, "migrations/versioned/000115_platform_chat_history.up.sql"),
		`INSERT INTO models(id, tenant_id, type, status, is_builtin) VALUES
			('embed-platform', 0, 'Embedding', 'active', TRUE)`,
		`UPDATE platform_chat_history_config
			SET enabled = TRUE, embedding_model_id = 'embed-platform' WHERE id = 1`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	return db, NewPlatformChatHistoryRepository(db)
}

func TestPostgresConcurrentFirstMessageIndexCreatesOnePrivateKnowledgeBase(t *testing.T) {
	db, repo := newPlatformChatHistoryPostgres(t)
	require.NoError(t, db.Exec(`INSERT INTO tenants(id) VALUES (10001)`).Error)

	const workers = 8
	start := make(chan struct{})
	results := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			kb, _, err := repo.EnsureTenantKnowledgeBase(t.Context(), "embed-platform", preparedChatHistoryKB(10001, "embed-platform"))
			if kb != nil {
				results <- kb.ID
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	oneID := ""
	for id := range results {
		if oneID == "" {
			oneID = id
		}
		require.Equal(t, oneID, id)
	}
	require.NotEmpty(t, oneID)
	var kbCount, bindingCount int64
	require.NoError(t, db.Table("knowledge_bases").Where("tenant_id = 10001").Count(&kbCount).Error)
	require.NoError(t, db.Table("tenant_chat_history_indexes").Where("tenant_id = 10001").Count(&bindingCount).Error)
	require.Equal(t, int64(1), kbCount)
	require.Equal(t, int64(1), bindingCount)
}

func TestPostgresMigrationRejectsLegacyKnowledgeBaseBinding(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_platform_chat_history_migration_")
	require.NoError(t, db.Exec(`CREATE TABLE tenants (
		id INTEGER PRIMARY KEY, chat_history_config JSONB, deleted_at TIMESTAMPTZ
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE knowledge_bases (id VARCHAR(36) PRIMARY KEY)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tenants(id, chat_history_config)
		VALUES (10001, '{"enabled":true,"knowledge_base_id":"legacy-kb"}'::jsonb)`).Error)
	require.ErrorContains(
		t,
		db.Exec(migrationSQL(t, "migrations/versioned/000115_platform_chat_history.up.sql")).Error,
		"legacy chat-history KB bindings require explicit cleanup",
	)
	var configTable *string
	require.NoError(t, db.Raw(`SELECT to_regclass('platform_chat_history_config')::text`).Scan(&configTable).Error)
	require.Nil(t, configTable)
}
