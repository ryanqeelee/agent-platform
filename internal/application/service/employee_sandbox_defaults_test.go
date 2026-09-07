package service

import (
	"context"
	"encoding/json"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmployeeDefaultPublishesPendingConfigWithoutLosingProvider(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	t.Chdir(filepath.Join("..", "..", ".."))
	fx := newInstallFixture(t)
	cfg := e2bCfg("key", "", "e2b.app", "base", 1800)
	cfg.SkillPreparation = "session"
	cfg.E2B.OnTimeout = "kill"
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	t.Setenv("WEKNORA_EMPLOYEE_SANDBOX_DEFAULT_JSON", string(raw))
	repo := &fakeConfigRepo{entity: &types.TenantSandboxConfigEntity{
		ID: "cfg-1", TenantID: 7, Name: "employee-assistant-initializing", Config: cfg,
	}}
	for _, name := range []string{"document-analyzer", "data-processor", "citation-generator", "doc-coauthoring"} {
		archive, err := builtinSkillArchive(filepath.Join("skills", "preloaded", name))
		require.NoError(t, err)
		bundle, err := ParseSkillBundle(archive)
		require.NoError(t, err)
		require.NoError(t, fx.skillRepo.CreateSkill(context.Background(), &types.TenantSkillEntity{
			ID: name, TenantID: 7, SandboxConfigID: "cfg-1", Name: bundle.Name, Status: types.SkillStatusReady,
		}))
	}
	agents := &customAgentService{}
	configs := NewTenantSandboxConfigService(repo, stubAgentRepo{}, testGlobalSandboxConfig(), nil, nil)
	require.NoError(t, ConfigureEmployeeSandboxDefaults(agents, configs, fx.svc))
	require.NoError(t, agents.provisionEmployeeSandbox(context.Background(), 7))
	require.Equal(t, "employee-assistant", repo.updated.Name)
	require.Equal(t, cfg, repo.updated.Config)
}

func TestEmployeeDefaultBundlesAreInstallableArchives(t *testing.T) {
	for _, name := range []string{"document-analyzer", "data-processor", "citation-generator", "doc-coauthoring"} {
		raw, err := builtinSkillArchive(filepath.Join("..", "..", "..", "skills", "preloaded", name))
		require.NoError(t, err)
		bundle, err := ParseSkillBundle(raw)
		require.NoError(t, err)
		require.NotEmpty(t, bundle.Name)
		require.NotEmpty(t, bundle.Instructions)
	}
}
