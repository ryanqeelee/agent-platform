package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type agentMemoryWebSearchRepo struct{}

func (*agentMemoryWebSearchRepo) Create(context.Context, *types.WebSearchProviderEntity) error {
	return nil
}
func (*agentMemoryWebSearchRepo) GetByID(context.Context, uint64, string) (*types.WebSearchProviderEntity, error) {
	return nil, nil
}
func (*agentMemoryWebSearchRepo) GetDefault(context.Context, uint64) (*types.WebSearchProviderEntity, error) {
	return nil, nil
}
func (*agentMemoryWebSearchRepo) List(context.Context, uint64) ([]*types.WebSearchProviderEntity, error) {
	return nil, nil
}
func (*agentMemoryWebSearchRepo) Update(context.Context, *types.WebSearchProviderEntity) error {
	return nil
}
func (*agentMemoryWebSearchRepo) Delete(context.Context, uint64, string) error       { return nil }
func (*agentMemoryWebSearchRepo) ClearDefault(context.Context, uint64, string) error { return nil }

// buildAgentConfig copies CustomAgentConfig into the runtime AgentConfig field
// by field, and MemoryEnabled was missing from that list. Nothing failed
// loudly: the agent path read a nil preference and treated every agent as
// inheriting the workspace, so an agent explicitly barred from memory still
// got the user's memories injected into its prompt.
func TestAgentConfigCarriesTheMemoryPreference(t *testing.T) {
	for _, tc := range []struct {
		name string
		want *bool
	}{
		{name: "opted out", want: boolPtr(false)},
		{name: "opted in", want: boolPtr(true)},
		{name: "inherits the workspace", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &sessionService{
				cfg:                   &config.Config{},
				webSearchProviderRepo: &agentMemoryWebSearchRepo{},
			}
			req := &types.QARequest{
				Session: &types.Session{ID: "session-1", TenantID: 1},
				CustomAgent: &types.CustomAgent{
					TenantID: 1,
					Config: types.CustomAgentConfig{
						MaxIterations: 5,
						MemoryEnabled: tc.want,
					},
				},
			}

			agentConfig, _, err := svc.buildAgentConfig(t.Context(), req, &types.Tenant{ID: 1}, 1)
			require.NoError(t, err)
			require.Equal(t, tc.want, agentConfig.MemoryEnabled)
		})
	}
}

func TestAgentConfigCarriesMaxCompletionTokens(t *testing.T) {
	svc := &sessionService{
		cfg:                   &config.Config{},
		webSearchProviderRepo: &agentMemoryWebSearchRepo{},
	}
	req := &types.QARequest{
		Session: &types.Session{ID: "session-1", TenantID: 1},
		CustomAgent: &types.CustomAgent{
			TenantID: 1,
			Config: types.CustomAgentConfig{
				MaxIterations:       5,
				MaxCompletionTokens: 64000,
			},
		},
	}

	agentConfig, _, err := svc.buildAgentConfig(t.Context(), req, &types.Tenant{ID: 1}, 1)
	require.NoError(t, err)
	require.Equal(t, 64000, agentConfig.MaxCompletionTokens)
}

func boolPtr(v bool) *bool { return &v }
