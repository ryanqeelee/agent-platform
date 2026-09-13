package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type platformAgentRepoStub struct {
	interfaces.CustomAgentRepository
	rows    map[uint64]map[string]*types.CustomAgent
	getErr  map[uint64]error
	created *types.CustomAgent
	updated *types.CustomAgent
}

func (r *platformAgentRepoStub) GetAgentByID(_ context.Context, id string, tenantID uint64) (*types.CustomAgent, error) {
	if err := r.getErr[tenantID]; err != nil {
		return nil, err
	}
	if row := r.rows[tenantID][id]; row != nil {
		copy := *row
		return &copy, nil
	}
	return nil, repository.ErrCustomAgentNotFound
}
func (r *platformAgentRepoStub) ListAgentsByTenantID(_ context.Context, tenantID uint64) ([]*types.CustomAgent, error) {
	result := make([]*types.CustomAgent, 0, len(r.rows[tenantID]))
	for _, row := range r.rows[tenantID] {
		copy := *row
		result = append(result, &copy)
	}
	return result, nil
}
func (r *platformAgentRepoStub) CreateAgent(_ context.Context, agent *types.CustomAgent) error {
	r.created = agent
	if r.rows == nil {
		r.rows = make(map[uint64]map[string]*types.CustomAgent)
	}
	if r.rows[agent.TenantID] == nil {
		r.rows[agent.TenantID] = make(map[string]*types.CustomAgent)
	}
	copy := *agent
	r.rows[agent.TenantID][agent.ID] = &copy
	return nil
}
func (r *platformAgentRepoStub) UpdateAgent(_ context.Context, agent *types.CustomAgent) error {
	r.updated = agent
	if r.rows == nil {
		r.rows = make(map[uint64]map[string]*types.CustomAgent)
	}
	if r.rows[agent.TenantID] == nil {
		r.rows[agent.TenantID] = make(map[string]*types.CustomAgent)
	}
	copy := *agent
	r.rows[agent.TenantID][agent.ID] = &copy
	return nil
}

type platformModelRepoStub struct {
	interfaces.ModelRepository
	models map[string]*types.Model
}

func (r platformModelRepoStub) GetByID(_ context.Context, _ uint64, id string) (*types.Model, error) {
	return r.models[id], nil
}

func platformAdminContext() context.Context {
	return context.WithValue(context.Background(), types.SystemAdminContextKey, true)
}

func installTestBuiltin(t *testing.T, id string, config types.CustomAgentConfig) {
	installTestBuiltins(t, map[string]types.CustomAgentConfig{id: config})
}

func installTestBuiltins(t *testing.T, configs map[string]types.CustomAgentConfig) {
	previousFactories := make(map[string]func(uint64) *types.CustomAgent, len(configs))
	existed := make(map[string]bool, len(configs))
	entries := make(map[string]*types.BuiltinAgentEntry, len(configs))
	for id, config := range configs {
		previousFactories[id], existed[id] = types.BuiltinAgentRegistry[id]
		id, config := id, config
		types.BuiltinAgentRegistry[id] = func(tenantID uint64) *types.CustomAgent {
			return &types.CustomAgent{ID: id, TenantID: tenantID, IsBuiltin: true, Name: id, Config: config}
		}
		entries[id] = &types.BuiltinAgentEntry{ID: id, IsBuiltin: true, I18n: map[string]types.BuiltinAgentI18n{
			"default": {Name: id},
		}, Config: config}
	}
	restore := types.OverrideBuiltinAgentEntriesForTest(entries)
	t.Cleanup(func() {
		restore()
		for id := range configs {
			if existed[id] {
				types.BuiltinAgentRegistry[id] = previousFactories[id]
			} else {
				delete(types.BuiltinAgentRegistry, id)
			}
		}
	})
}

func TestPlatformAgentManagementIsTenantlessAndExcludesInstaller(t *testing.T) {
	installTestBuiltin(t, types.BuiltinQuickAnswerID, types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer})
	repo := &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{}}
	svc := NewPlatformAgentService(repo, platformModelRepoStub{})

	_, err := svc.List(context.Background())
	require.ErrorIs(t, err, ErrPlatformAgentForbidden)

	agents, err := svc.List(platformAdminContext())
	require.NoError(t, err)
	require.NotEmpty(t, agents)
	for _, agent := range agents {
		require.Zero(t, agent.TenantID)
		require.NotEqual(t, types.BuiltinSkillInstallerID, agent.ID)
	}
	_, err = svc.Get(platformAdminContext(), types.BuiltinSkillInstallerID)
	require.ErrorIs(t, err, ErrPlatformAgentNotFound)
}

func TestPlatformAgentUpdateRejectsTenantBindingsAndNonGlobalModels(t *testing.T) {
	installTestBuiltin(t, types.BuiltinQuickAnswerID, types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer})
	repo := &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{}}
	svc := NewPlatformAgentService(repo, platformModelRepoStub{models: map[string]*types.Model{
		"tenant-chat": {ID: "tenant-chat", TenantID: 7, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
	}})

	for _, cfg := range []types.CustomAgentConfig{
		{KnowledgeBases: []string{"kb-7"}},
		{KBSelectionMode: "selected"},
		{SandboxConfigID: "sandbox-7"},
		{WebSearchProviderID: "provider-7"},
		{MCPServices: []string{"mcp-7"}},
		{SelectedSkills: []string{"tenant-skill"}},
		{ModelID: "tenant-chat"},
	} {
		_, err := svc.Update(platformAdminContext(), types.BuiltinQuickAnswerID, cfg)
		require.ErrorIs(t, err, ErrPlatformAgentInvalidConfig)
	}
	require.Nil(t, repo.created)
}

func TestPlatformAgentUpdatePersistsGlobalConfig(t *testing.T) {
	installTestBuiltin(t, types.BuiltinQuickAnswerID, types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer})
	repo := &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{}}
	svc := NewPlatformAgentService(repo, platformModelRepoStub{models: map[string]*types.Model{
		"global-chat": {ID: "global-chat", TenantID: 0, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
	}})

	updated, err := svc.Update(platformAdminContext(), types.BuiltinQuickAnswerID, types.CustomAgentConfig{
		AgentMode:       types.AgentModeQuickAnswer,
		ModelID:         "global-chat",
		SystemPrompt:    "platform prompt",
		KBSelectionMode: "all",
	})
	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Zero(t, repo.created.TenantID)
	require.True(t, repo.created.IsBuiltin)
	require.Equal(t, "platform prompt", updated.Config.SystemPrompt)

	runtime := &customAgentService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(91))
	observed, err := runtime.GetAgentByID(ctx, types.BuiltinQuickAnswerID)
	require.NoError(t, err)
	require.Equal(t, "platform prompt", observed.Config.SystemPrompt)
	require.Equal(t, uint64(91), observed.TenantID)
}

func TestRuntimeUsesGlobalBuiltinAndIgnoresTenantOverride(t *testing.T) {
	installTestBuiltins(t, map[string]types.CustomAgentConfig{
		types.BuiltinQuickAnswerID:       {AgentMode: types.AgentModeQuickAnswer},
		types.BuiltinEmployeeAssistantID: {AgentMode: types.AgentModeSmartReasoning, SkillsSelectionMode: "none"},
	})
	repo := &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{
		0: {
			types.BuiltinQuickAnswerID: {ID: types.BuiltinQuickAnswerID, IsBuiltin: true, Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer, SystemPrompt: "global"}},
		},
		7: {
			types.BuiltinQuickAnswerID: {ID: types.BuiltinQuickAnswerID, TenantID: 7, IsBuiltin: true, Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer, SystemPrompt: "old tenant override"}},
		},
	}}
	svc := &customAgentService{repo: repo, scenarioCapabilities: assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, false)}}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	agent, err := svc.GetAgentByID(ctx, types.BuiltinQuickAnswerID)
	require.NoError(t, err)
	require.Equal(t, "global", agent.Config.SystemPrompt)
	require.Equal(t, uint64(7), agent.TenantID)

	agents, err := svc.ListAgents(ctx)
	require.NoError(t, err)
	for _, listed := range agents {
		if listed.ID == types.BuiltinQuickAnswerID {
			require.Equal(t, "global", listed.Config.SystemPrompt)
		}
	}

}

func TestEmployeeRuntimeKeepsGlobalPolicyAndInjectsEnterpriseBindings(t *testing.T) {
	installTestBuiltin(t, types.BuiltinEmployeeAssistantID, types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning})
	repo := &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{
		0: {
			types.BuiltinEmployeeAssistantID: {
				ID: types.BuiltinEmployeeAssistantID, IsBuiltin: true,
				Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning, WebSearchEnabled: false, SkillsSelectionMode: "all"},
			},
		},
	}}
	svc := &customAgentService{
		repo:                 repo,
		scenarioCapabilities: assistantScenarioResolverStub{settings: assistantScenarioSettings(true, false, true)},
		sandboxConfigs:       &employeeSandboxRepoStub{rows: []*types.TenantSandboxConfigEntity{{ID: "tenant-sandbox", TenantID: 7, Name: "employee-assistant"}}},
		webSearchProviders:   &employeeProviderStub{provider: &types.WebSearchProviderEntity{ID: "tenant-provider", TenantID: 7, Provider: types.WebSearchProviderTypeKeenable}},
	}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	agent, err := svc.GetAgentByID(ctx, types.BuiltinEmployeeAssistantID)
	require.NoError(t, err)
	require.False(t, agent.Config.WebSearchEnabled)
	require.Empty(t, agent.Config.WebSearchProviderID)
	require.Equal(t, "tenant-sandbox", agent.Config.SandboxConfigID)
	require.Equal(t, "all", agent.Config.SkillsSelectionMode)
}

func TestSkillInstallerUpdateAndReadStayTenantScoped(t *testing.T) {
	installTestBuiltin(t, types.BuiltinSkillInstallerID, types.CustomAgentConfig{ModelID: "yaml-model"})
	repo := &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{
		42: {
			types.BuiltinSkillInstallerID: {
				ID: types.BuiltinSkillInstallerID, TenantID: 42, IsBuiltin: true,
				Config: types.CustomAgentConfig{ModelID: "model-before"},
			},
		},
	}}
	svc := &customAgentService{repo: repo}
	ctx42 := context.WithValue(platformAdminContext(), types.TenantIDContextKey, uint64(42))

	updated, err := svc.UpdateAgent(ctx42, &types.CustomAgent{
		ID:     types.BuiltinSkillInstallerID,
		Config: types.CustomAgentConfig{ModelID: "model-after"},
	})
	require.NoError(t, err)
	require.Equal(t, "model-after", updated.Config.ModelID)

	observed, err := svc.GetAgentByID(ctx42, types.BuiltinSkillInstallerID)
	require.NoError(t, err)
	require.Equal(t, uint64(42), observed.TenantID)
	require.True(t, observed.IsBuiltin)
	require.Equal(t, "model-after", observed.Config.ModelID)

	ctx43 := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(43))
	fallback, err := svc.GetAgentByID(ctx43, types.BuiltinSkillInstallerID)
	require.NoError(t, err)
	require.Equal(t, uint64(43), fallback.TenantID)
	require.True(t, fallback.IsBuiltin)
	require.Equal(t, "yaml-model", fallback.Config.ModelID)
}

func TestSkillInstallerReadPropagatesRepositoryErrors(t *testing.T) {
	installTestBuiltin(t, types.BuiltinSkillInstallerID, types.CustomAgentConfig{ModelID: "yaml-model"})
	storageErr := errors.New("storage unavailable")
	svc := &customAgentService{repo: &platformAgentRepoStub{
		rows:   map[uint64]map[string]*types.CustomAgent{},
		getErr: map[uint64]error{42: storageErr},
	}}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(42))

	_, err := svc.GetAgentByID(ctx, types.BuiltinSkillInstallerID)
	require.ErrorIs(t, err, storageErr)
}
