package service

import (
	"context"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type preparationSessions struct {
	interfaces.SessionService
	tenant uint64
}

func (s preparationSessions) GetSession(_ context.Context, id string) (*types.Session, error) {
	return &types.Session{ID: id, TenantID: s.tenant, SandboxConfigID: "cfg-1"}, nil
}

type preparationSandbox struct {
	*installSandboxManager
	marker string
	digest string
}

func (s *preparationSandbox) SessionInstallShellExecutor() sandbox.SessionInstallShellExecutor {
	return s
}
func (s *preparationSandbox) ExecShellCommandWithOptions(ctx context.Context, session, command string, opts sandbox.ShellExecOptions) (*sandbox.ExecuteResult, error) {
	if strings.HasPrefix(command, "if test -f ") {
		return &sandbox.ExecuteResult{Stdout: s.marker}, nil
	}
	if strings.HasPrefix(command, "printf %s ") {
		s.marker = s.digest
		return &sandbox.ExecuteResult{}, nil
	}
	return s.installSandboxManager.ExecShellCommandWithOptions(ctx, session, command, opts)
}
func TestSessionPreparationReusesInstanceAndReinstallsAfterRecreation(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillPreparation = "session"
	fx.svc.sessions = preparationSessions{fx.svc.sessions, 7}
	archive := zipBundle(t, map[string]string{"SKILL.md": validSkillMD, "scripts/extract.py": "print('hi')\n"})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	row, err := fx.skillRepo.GetSkill(context.Background(), 7, "cfg-1", "sk-1")
	require.NoError(t, err)
	row.Status = types.SkillStatusReady
	row.BundleSHA256 = bundle.SHA256
	row.BundleRef = "file://runtime.zip"
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), row))
	fx.storedBundles = map[string][]byte{row.BundleRef: archive}
	mgr := &preparationSandbox{installSandboxManager: fx.sandboxMgr, digest: row.BundleSHA256}
	prepare := func() error {
		return fx.svc.prepareSessionSkill(context.Background(), mgr, 7, "user-session", "cfg-1", row)
	}
	require.NoError(t, prepare())
	require.Len(t, fx.agentPrompts, 1)
	require.Equal(t, row.BundleSHA256, mgr.marker)
	require.NoError(t, prepare())
	require.Len(t, fx.agentPrompts, 1)
	mgr.marker = "" // a recreated instance has no completion file
	require.NoError(t, prepare())
	require.Len(t, fx.agentPrompts, 2)
	require.NotContains(t, fx.events, "create-snapshot")
	mgr.marker = ""
	fx.agentErr = errAgentBoom
	require.ErrorContains(t, prepare(), "agent boom")
	require.Empty(t, mgr.marker)
	current, err := fx.skillRepo.GetSkill(context.Background(), 7, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, current.Status)
	fx.agentErr = nil

	fx.svc.sessions = preparationSessions{fx.svc.sessions, 8}
	require.ErrorContains(t, prepare(), "outside the workspace")
	require.Len(t, fx.agentPrompts, 3)
}
func TestSessionPreparationFailureDoesNotRecordCompletion(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillPreparation = "session"
	fx.svc.sessions = preparationSessions{fx.svc.sessions, 7}
	row, err := fx.skillRepo.GetSkill(context.Background(), 7, "cfg-1", "sk-1")
	require.NoError(t, err)
	row.Status = types.SkillStatusReady
	row.BundleRef = "file://absent.zip"
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), row))
	mgr := &preparationSandbox{installSandboxManager: fx.sandboxMgr, digest: row.BundleSHA256}
	require.Error(t, fx.svc.prepareSessionSkill(context.Background(), mgr, 7, "user-session", "cfg-1", row))
	require.Empty(t, mgr.marker)
	require.Empty(t, fx.agentPrompts)
	row.Enabled = false
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), row))
	require.ErrorContains(t, fx.svc.prepareSessionSkill(context.Background(), mgr, 7, "user-session", "cfg-1", row), "disabled")
}
