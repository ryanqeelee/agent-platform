package repository

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupModelCredentialDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:model-credentials-" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}, &types.PlatformMemoryRuntimeConfig{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE platform_chat_history_config (id INTEGER PRIMARY KEY CHECK (id = 1), enabled BOOLEAN NOT NULL DEFAULT 0, embedding_model_id TEXT NOT NULL);
		CREATE TABLE knowledge_bases (id TEXT PRIMARY KEY, embedding_model_id TEXT NOT NULL);
		CREATE TABLE tenant_chat_history_indexes (tenant_id INTEGER PRIMARY KEY, knowledge_base_id TEXT NOT NULL UNIQUE)
	`).Error)
	require.NoError(t, db.Create(&types.PlatformMemoryRuntimeConfig{
		ID: types.PlatformMemoryRuntimeConfigSingletonID, Runtime: types.DefaultMemoryRuntimeConfig(),
		UpdatedBy: "test",
	}).Error)
	return db
}

func rawModelParameters(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var raw string
	require.NoError(t, db.Raw("SELECT parameters FROM models WHERE id = ?", id).Scan(&raw).Error)
	return raw
}

func modelCredentialFixture(id string) *types.Model {
	return &types.Model{
		ID: id, TenantID: 1, Name: id,
		Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
		Status: types.ModelStatusActive,
	}
}

func TestModelRepositoryEncryptsCredentialsAtRest(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	db := setupModelCredentialDB(t)
	repo := NewModelRepository(db)
	model := modelCredentialFixture("encrypted-model")
	model.Parameters.APIKey = "api-secret-value"
	model.Parameters.AppSecret = "app-secret-value"

	require.NoError(t, repo.Create(context.Background(), model))
	raw := rawModelParameters(t, db, model.ID)
	assert.Contains(t, raw, utils.EncPrefix)
	assert.NotContains(t, raw, "api-secret-value")
	assert.NotContains(t, raw, "app-secret-value")
}

func TestModelRepositoryRefusesCredentialWritesWithoutValidAESKey(t *testing.T) {
	for _, key := range []string{"", "wrong-length"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("SYSTEM_AES_KEY", key)
			db := setupModelCredentialDB(t)
			repo := NewModelRepository(db)

			withSecret := modelCredentialFixture("rejected-create")
			withSecret.Parameters.APIKey = "must-not-be-written"
			err := repo.Create(context.Background(), withSecret)
			require.Error(t, err)
			assert.True(t, stderrors.Is(err, types.ErrModelCredentialEncryptionUnavailable))
			var count int64
			require.NoError(t, db.Model(&types.Model{}).Where("id = ?", withSecret.ID).Count(&count).Error)
			assert.Zero(t, count)

			withoutSecret := modelCredentialFixture("safe-model")
			require.NoError(t, repo.Create(context.Background(), withoutSecret))
			before := rawModelParameters(t, db, withoutSecret.ID)
			withoutSecret.Parameters.AppSecret = "must-not-replace-row"
			err = repo.Update(context.Background(), withoutSecret)
			require.Error(t, err)
			assert.True(t, stderrors.Is(err, types.ErrModelCredentialEncryptionUnavailable))
			after := rawModelParameters(t, db, withoutSecret.ID)
			assert.Equal(t, before, after)
		})
	}
}

func TestModelRepositoryStillReadsLegacyPlaintextCredentials(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")
	db := setupModelCredentialDB(t)
	repo := NewModelRepository(db)
	model := modelCredentialFixture("legacy-model")
	require.NoError(t, repo.Create(context.Background(), model))
	require.NoError(t, db.Exec(
		"UPDATE models SET parameters = ? WHERE id = ?",
		[]byte(`{"api_key":"legacy-plain-value","app_secret":"legacy-app-value"}`),
		model.ID,
	).Error)

	loaded, err := repo.GetByID(context.Background(), model.TenantID, model.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "legacy-plain-value", loaded.Parameters.APIKey)
	assert.Equal(t, "legacy-app-value", loaded.Parameters.AppSecret)
}
