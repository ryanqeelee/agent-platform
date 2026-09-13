package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newPlatformMemoryRuntimeServiceTest(t *testing.T) (*gorm.DB, *PlatformMemoryRuntimeConfigService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Model{}, &types.PlatformMemoryRuntimeConfig{}))
	require.NoError(t, db.Create(&types.PlatformMemoryRuntimeConfig{
		ID: types.PlatformMemoryRuntimeConfigSingletonID, Runtime: types.DefaultMemoryRuntimeConfig(),
		UpdatedBy: "migration",
	}).Error)
	return db, NewPlatformMemoryRuntimeConfigService(repository.NewPlatformMemoryRuntimeConfigRepository(db))
}

func platformMemoryRuntimeActorContext() context.Context {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "platform-admin")
	return context.WithValue(ctx, types.SystemAdminContextKey, true)
}

func TestPlatformMemoryRuntimeUpdateValidatesPlatformModelsWithoutTenants(t *testing.T) {
	db, svc := newPlatformMemoryRuntimeServiceTest(t)
	for _, model := range []*types.Model{
		{ID: "extract", TenantID: 0, Name: "extract", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote, IsBuiltin: true, Status: types.ModelStatusActive},
		{ID: "embed", TenantID: 0, Name: "embed", Type: types.ModelTypeEmbedding, Source: types.ModelSourceRemote, IsBuiltin: true, Status: types.ModelStatusActive},
	} {
		require.NoError(t, db.Create(model).Error)
	}
	runtime := types.DefaultMemoryRuntimeConfig()
	runtime.ExtractModelID = "extract"
	runtime.EmbeddingModelID = "embed"
	runtime.MaxItems = 321
	updated, err := svc.Update(platformMemoryRuntimeActorContext(), runtime)
	require.NoError(t, err)
	require.Equal(t, "extract", updated.ExtractModelID)
	require.Equal(t, "embed", updated.EmbeddingModelID)
	require.Equal(t, 321, updated.MaxItems)
}

func TestPlatformMemoryRuntimeUpdateRejectsInvalidModelBindings(t *testing.T) {
	tests := []struct {
		name  string
		model *types.Model
		bind  func(*types.MemoryRuntimeConfig)
	}{
		{name: "missing", bind: func(c *types.MemoryRuntimeConfig) { c.EmbeddingModelID = "missing" }},
		{name: "enterprise owned", model: &types.Model{ID: "candidate", TenantID: 7, IsBuiltin: false, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive}, bind: func(c *types.MemoryRuntimeConfig) { c.EmbeddingModelID = "candidate" }},
		{name: "inactive", model: &types.Model{ID: "candidate", IsBuiltin: true, Type: types.ModelTypeEmbedding, Status: types.ModelStatusDownloadFailed}, bind: func(c *types.MemoryRuntimeConfig) { c.EmbeddingModelID = "candidate" }},
		{name: "wrong type", model: &types.Model{ID: "candidate", IsBuiltin: true, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive}, bind: func(c *types.MemoryRuntimeConfig) { c.EmbeddingModelID = "candidate" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, svc := newPlatformMemoryRuntimeServiceTest(t)
			if test.model != nil {
				test.model.Name = test.model.ID
				test.model.Source = types.ModelSourceRemote
				require.NoError(t, db.Create(test.model).Error)
			}
			runtime := types.DefaultMemoryRuntimeConfig()
			test.bind(runtime)
			_, err := svc.Update(platformMemoryRuntimeActorContext(), runtime)
			require.Error(t, err)
			appErr, ok := apperrors.IsAppError(err)
			require.True(t, ok)
			require.Equal(t, apperrors.ErrValidation, appErr.Code)
		})
	}
}

func TestPlatformMemoryRuntimeMissingSingletonIsUnavailable(t *testing.T) {
	db, svc := newPlatformMemoryRuntimeServiceTest(t)
	require.NoError(t, db.Exec("DELETE FROM platform_memory_runtime_config").Error)
	_, err := svc.Get(context.Background())
	require.ErrorContains(t, err, "singleton is missing")
	_, err = svc.Update(platformMemoryRuntimeActorContext(), types.DefaultMemoryRuntimeConfig())
	require.Error(t, err)
}

func TestPlatformMemoryRuntimeUpdateRequiresSystemAdministrator(t *testing.T) {
	_, svc := newPlatformMemoryRuntimeServiceTest(t)
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "enterprise-admin")
	_, err := svc.Update(ctx, types.DefaultMemoryRuntimeConfig())
	require.Error(t, err)
	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.ErrForbidden, appErr.Code)
}
