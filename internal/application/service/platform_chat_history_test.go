package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type platformChatHistoryRepoStub struct {
	config      types.PlatformChatHistoryConfig
	bindings    map[uint64]*types.KnowledgeBase
	updateCalls int
}

func (s *platformChatHistoryRepoStub) GetConfig(context.Context) (*types.PlatformChatHistoryConfig, error) {
	copy := s.config
	return &copy, nil
}

func (s *platformChatHistoryRepoStub) UpdateConfig(
	_ context.Context, config *types.PlatformChatHistoryConfig, actorID string, now time.Time,
) (*types.PlatformChatHistoryConfig, error) {
	s.updateCalls++
	s.config.Enabled = config.Enabled
	s.config.EmbeddingModelID = config.EmbeddingModelID
	s.config.UpdatedBy = actorID
	s.config.UpdatedAt = now
	copy := s.config
	return &copy, nil
}

func (s *platformChatHistoryRepoStub) GetTenantKnowledgeBase(_ context.Context, tenantID uint64) (*types.KnowledgeBase, error) {
	if kb := s.bindings[tenantID]; kb != nil {
		copy := *kb
		return &copy, nil
	}
	return nil, nil
}

func (*platformChatHistoryRepoStub) EnsureTenantKnowledgeBase(context.Context, string, *types.KnowledgeBase) (*types.KnowledgeBase, bool, error) {
	panic("unexpected EnsureTenantKnowledgeBase call")
}

func (s *platformChatHistoryRepoStub) Stats(_ context.Context, tenantID *uint64) (int64, int64, error) {
	if tenantID == nil {
		return int64(len(s.bindings)), 0, nil
	}
	if s.bindings[*tenantID] != nil {
		return 1, 0, nil
	}
	return 0, 0, nil
}

type platformChatHistoryModelStub struct {
	interfaces.ModelService
	models map[string]*types.Model
}

func (s platformChatHistoryModelStub) GetModelByID(_ context.Context, id string) (*types.Model, error) {
	return s.models[id], nil
}

func platformChatHistoryAdminContext() context.Context {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "platform-admin")
	return context.WithValue(ctx, types.SystemAdminContextKey, true)
}

func TestPlatformChatHistoryUpdateValidatesEnabledModel(t *testing.T) {
	models := map[string]*types.Model{
		"embedding":    {ID: "embedding", TenantID: 0, IsBuiltin: true, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive},
		"wrong-type":   {ID: "wrong-type", TenantID: 0, IsBuiltin: true, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
		"wrong-status": {ID: "wrong-status", TenantID: 0, IsBuiltin: true, Type: types.ModelTypeEmbedding, Status: types.ModelStatusDownloading},
		"wrong-scope":  {ID: "wrong-scope", TenantID: 10001, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive},
		"tenant-zero":  {ID: "tenant-zero", TenantID: 0, Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive},
	}
	tests := []struct {
		name   string
		config types.PlatformChatHistoryConfig
	}{
		{name: "enabled blank", config: types.PlatformChatHistoryConfig{Enabled: true}},
		{name: "missing", config: types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: "missing"}},
		{name: "wrong type", config: types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: "wrong-type"}},
		{name: "wrong status", config: types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: "wrong-status"}},
		{name: "wrong scope", config: types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: "wrong-scope"}},
		{name: "tenant zero without platform visibility", config: types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: "tenant-zero"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &platformChatHistoryRepoStub{}
			service := NewPlatformChatHistoryService(repo, platformChatHistoryModelStub{models: models})
			_, err := service.Update(platformChatHistoryAdminContext(), &test.config)
			require.Error(t, err)
			require.Zero(t, repo.updateCalls)
		})
	}

	repo := &platformChatHistoryRepoStub{}
	service := NewPlatformChatHistoryService(repo, platformChatHistoryModelStub{models: models})
	updated, err := service.Update(platformChatHistoryAdminContext(), &types.PlatformChatHistoryConfig{Enabled: false})
	require.NoError(t, err)
	require.False(t, updated.Enabled)
	require.Empty(t, updated.EmbeddingModelID)
	updated, err = service.Update(platformChatHistoryAdminContext(), &types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: " embedding "})
	require.NoError(t, err)
	require.True(t, updated.Enabled)
	require.Equal(t, "embedding", updated.EmbeddingModelID)
}

func TestPlatformChatHistoryUpdateRequiresPlatformAdministrator(t *testing.T) {
	repo := &platformChatHistoryRepoStub{}
	service := NewPlatformChatHistoryService(repo, platformChatHistoryModelStub{})
	_, err := service.Update(context.Background(), &types.PlatformChatHistoryConfig{})
	require.Error(t, err)
	require.Zero(t, repo.updateCalls)
}

var _ repository.PlatformChatHistoryRepository = (*platformChatHistoryRepoStub)(nil)
