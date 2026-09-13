package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPlatformMemoryModelTest(t *testing.T, modelType types.ModelType) (*gorm.DB, *types.Model) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}, &types.PlatformMemoryRuntimeConfig{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE platform_chat_history_config (id INTEGER PRIMARY KEY CHECK (id = 1), enabled BOOLEAN NOT NULL DEFAULT 0, embedding_model_id TEXT NOT NULL);
		CREATE TABLE knowledge_bases (id TEXT PRIMARY KEY, embedding_model_id TEXT NOT NULL);
		CREATE TABLE tenant_chat_history_indexes (tenant_id INTEGER PRIMARY KEY, knowledge_base_id TEXT NOT NULL UNIQUE)
	`).Error)
	model := &types.Model{
		ID: "platform-model", TenantID: 0, Name: "remote-model", DisplayName: "Display",
		Type: modelType, Source: types.ModelSourceRemote, Description: "before",
		IsBuiltin: true, Status: types.ModelStatusActive,
		Parameters: types.ModelParameters{
			BaseURL: "https://models.invalid/v1", InterfaceType: "openai",
			EmbeddingParameters: types.EmbeddingParameters{Dimension: 768},
			Provider:            "provider-a", ExtraConfig: map[string]string{"route": "a"},
			CustomHeaders: map[string]string{"X-Route": "a"}, AppID: "app-a",
		},
	}
	require.NoError(t, db.Create(model).Error)
	runtime := types.DefaultMemoryRuntimeConfig()
	if modelType == types.ModelTypeEmbedding {
		runtime.EmbeddingModelID = model.ID
	} else {
		runtime.ExtractModelID = model.ID
	}
	require.NoError(t, db.Create(&types.PlatformMemoryRuntimeConfig{
		ID: types.PlatformMemoryRuntimeConfigSingletonID, Runtime: runtime, UpdatedBy: "test",
	}).Error)
	return db, model
}

func TestBoundPlatformEmbeddingModelRejectsVectorSpaceChanges(t *testing.T) {
	mutations := map[string]func(*types.Model){
		"name":                 func(m *types.Model) { m.Name = "other-model" },
		"type":                 func(m *types.Model) { m.Type = types.ModelTypeKnowledgeQA },
		"source":               func(m *types.Model) { m.Source = types.ModelSourceOpenAI },
		"base url":             func(m *types.Model) { m.Parameters.BaseURL = "https://other.invalid/v1" },
		"interface type":       func(m *types.Model) { m.Parameters.InterfaceType = "other" },
		"embedding parameters": func(m *types.Model) { m.Parameters.EmbeddingParameters.Dimension = 1536 },
		"provider":             func(m *types.Model) { m.Parameters.Provider = "provider-b" },
		"extra config":         func(m *types.Model) { m.Parameters.ExtraConfig["route"] = "b" },
		"custom headers":       func(m *types.Model) { m.Parameters.CustomHeaders["X-Route"] = "b" },
		"app id":               func(m *types.Model) { m.Parameters.AppID = "app-b" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			db, stored := newPlatformMemoryModelTest(t, types.ModelTypeEmbedding)
			incoming := *stored
			incoming.Parameters.ExtraConfig = map[string]string{"route": "a"}
			incoming.Parameters.CustomHeaders = map[string]string{"X-Route": "a"}
			mutate(&incoming)
			err := NewModelRepository(db).Update(context.Background(), &incoming)
			require.ErrorIs(t, err, types.ErrModelPlatformRuntimeBinding)
		})
	}
}

func TestBoundPlatformEmbeddingModelAllowsMetadataAndCredentialChanges(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "0123456789abcdef0123456789abcdef")
	db, stored := newPlatformMemoryModelTest(t, types.ModelTypeEmbedding)
	incoming := *stored
	incoming.DisplayName = "Renamed display"
	incoming.Description = "after"
	incoming.Parameters.SupportsVision = true
	incoming.Parameters.ContextWindow = 8192
	incoming.Parameters.MaxOutputTokens = 1024
	incoming.Parameters.MaxConcurrency = 9
	incoming.Parameters.APIKey = "rotated-secret"
	incoming.Parameters.AppSecret = "rotated-app-secret"

	require.NoError(t, NewModelRepository(db).Update(context.Background(), &incoming))
	var reloaded types.Model
	require.NoError(t, db.First(&reloaded, "id = ?", stored.ID).Error)
	require.Equal(t, "Renamed display", reloaded.DisplayName)
	require.Equal(t, "rotated-secret", reloaded.Parameters.APIKey)
}

func TestBoundPlatformModelsMustRemainActiveAndPlatformOwned(t *testing.T) {
	for _, modelType := range []types.ModelType{types.ModelTypeEmbedding, types.ModelTypeKnowledgeQA} {
		for _, change := range []struct {
			name   string
			mutate func(*types.Model)
		}{
			{name: "inactive", mutate: func(m *types.Model) { m.Status = types.ModelStatusDownloadFailed }},
			{name: "not platform owned", mutate: func(m *types.Model) { m.IsBuiltin = false }},
		} {
			t.Run(string(modelType)+"/"+change.name, func(t *testing.T) {
				db, stored := newPlatformMemoryModelTest(t, modelType)
				incoming := *stored
				change.mutate(&incoming)
				err := NewModelRepository(db).Update(context.Background(), &incoming)
				require.ErrorIs(t, err, types.ErrModelPlatformRuntimeBinding)
			})
		}
	}
}

func TestBoundExtractionModelAllowsChatIdentityChangeButNotTypeChange(t *testing.T) {
	db, stored := newPlatformMemoryModelTest(t, types.ModelTypeKnowledgeQA)
	incoming := *stored
	incoming.Name = "replacement-chat-model"
	incoming.Parameters.BaseURL = "https://replacement.invalid/v1"
	require.NoError(t, NewModelRepository(db).Update(context.Background(), &incoming))

	incoming.Type = types.ModelTypeEmbedding
	err := NewModelRepository(db).Update(context.Background(), &incoming)
	require.ErrorIs(t, err, types.ErrModelPlatformRuntimeBinding)
}

func TestPlatformMemoryBindingDirectlyGuardsDeleteWithNoTenants(t *testing.T) {
	db, model := newPlatformMemoryModelTest(t, types.ModelTypeEmbedding)
	repo := NewModelRepository(db)
	bindings, err := repo.PlatformMemoryModelBindings(context.Background(), model.ID)
	require.NoError(t, err)
	require.Equal(t, []types.ModelUsageBinding{types.ModelUsageBindingEmbeddingModel}, bindings)

	err = repo.Delete(context.Background(), model.TenantID, model.ID)
	require.True(t, errors.Is(err, types.ErrModelPlatformRuntimeBinding))
}

func TestChatHistoryEmbeddingBindingsGuardSemanticUpdateAndDelete(t *testing.T) {
	for _, binding := range []string{"platform disabled", "tenant index"} {
		t.Run(binding, func(t *testing.T) {
			db, model := newPlatformMemoryModelTest(t, types.ModelTypeEmbedding)
			runtime := types.DefaultMemoryRuntimeConfig()
			require.NoError(t, db.Model(&types.PlatformMemoryRuntimeConfig{}).
				Where("id = ?", types.PlatformMemoryRuntimeConfigSingletonID).
				Update("runtime", runtime).Error)
			if binding == "platform disabled" {
				require.NoError(t, db.Exec(
					"INSERT INTO platform_chat_history_config(id, enabled, embedding_model_id) VALUES (1, false, ?)", model.ID,
				).Error)
			} else {
				require.NoError(t, db.Exec(
					"INSERT INTO knowledge_bases(id, embedding_model_id) VALUES ('chat-kb', ?)", model.ID,
				).Error)
				require.NoError(t, db.Exec(
					"INSERT INTO tenant_chat_history_indexes(tenant_id, knowledge_base_id) VALUES (7, 'chat-kb')",
				).Error)
			}

			incoming := *model
			incoming.Name = "different-vector-model"
			err := NewModelRepository(db).Update(context.Background(), &incoming)
			require.ErrorIs(t, err, types.ErrModelPlatformRuntimeBinding)
			err = NewModelRepository(db).Delete(context.Background(), model.TenantID, model.ID)
			require.ErrorIs(t, err, types.ErrModelPlatformRuntimeBinding)
		})
	}
}
