package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type preparationSessions struct {
	interfaces.SessionRepository
	tenant  uint64
	created []*types.Session
}

func (s *preparationSessions) GetByID(_ context.Context, _ uint64, id string) (*types.Session, error) {
	return &types.Session{ID: id, TenantID: s.tenant, SandboxConfigID: "cfg-1"}, nil
}

func (s *preparationSessions) Create(
	_ context.Context, session *types.Session,
) (*types.Session, error) {
	cp := *session
	cp.ID = fmt.Sprintf("maintenance-%d", len(s.created)+1)
	s.created = append(s.created, &cp)
	return &cp, nil
}

type preparationMessages struct {
	interfaces.MessageRepository
	created []*types.Message
	updated []*types.Message
}

func (m *preparationMessages) CreateMessage(
	_ context.Context, message *types.Message,
) (*types.Message, error) {
	cp := *message
	m.created = append(m.created, &cp)
	return &cp, nil
}

func (m *preparationMessages) UpdateMessage(_ context.Context, message *types.Message) error {
	cp := *message
	m.updated = append(m.updated, &cp)
	return nil
}

type preparationSandbox struct {
	*installSandboxManager
	marker     string
	digest     string
	shellCalls int
}

func (s *preparationSandbox) SessionInstallShellExecutor() sandbox.SessionInstallShellExecutor {
	return s
}
func (s *preparationSandbox) ExecShellCommandWithOptions(ctx context.Context, session, command string, opts sandbox.ShellExecOptions) (*sandbox.ExecuteResult, error) {
	s.shellCalls++
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
	sessions := &preparationSessions{SessionRepository: fx.svc.sessions, tenant: 7}
	fx.svc.sessions = sessions
	fx.svc.messages = &preparationMessages{}
	archive := zipBundle(t, map[string]string{"SKILL.md": validSkillMD, "scripts/extract.py": "print('hi')\n"})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	row, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	row.Status = types.SkillStatusReady
	row.BundleSHA256 = bundle.SHA256
	row.BundleRef = "storage://runtime.zip"
	row.CatalogID = "catalog-runtime"
	require.NoError(t, fx.skillRepo.CreateCatalog(context.Background(), &types.TenantSkillCatalogEntity{
		ID:           row.CatalogID,
		Name:         row.Name,
		BundleRef:    row.BundleRef,
		BundleSHA256: row.BundleSHA256,
	}))
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), row))
	fx.storedBundles = map[string][]byte{row.BundleRef: archive}
	mgr := &preparationSandbox{installSandboxManager: fx.sandboxMgr, digest: row.BundleSHA256}
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user-1")
	prepare := func() error {
		return fx.svc.prepareSessionSkill(ctx, mgr, 7, "user-session", "cfg-1", row, true)
	}
	validateRead := func() error {
		return fx.svc.prepareSessionSkill(ctx, mgr, 7, "user-session", "cfg-1", row, false)
	}
	require.NoError(t, validateRead())
	require.Zero(t, mgr.shellCalls, "reading instructions must not even inspect a sandbox marker")
	require.Empty(t, fx.agentPrompts, "reading instructions must not invoke the installer agent")
	require.NoError(t, prepare())
	require.Len(t, fx.agentPrompts, 1)
	require.Equal(t, row.BundleSHA256, mgr.marker)
	shellCalls := mgr.shellCalls
	require.NoError(t, validateRead())
	require.Equal(t, shellCalls, mgr.shellCalls)
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
	require.Len(t, sessions.created, 3)
	for _, created := range sessions.created {
		require.Equal(t, uint64(7), created.TenantID)
		require.Equal(t, "user-1", created.UserID)
		require.True(t, strings.HasPrefix(created.Description, types.SkillMaintenanceSessionMarker))
	}
	current, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, current.Status)
	fx.agentErr = nil

	fx.svc.sessions = &preparationSessions{SessionRepository: fx.svc.sessions, tenant: 8}
	require.ErrorContains(t, prepare(), "outside the workspace")
	require.ErrorContains(t, validateRead(), "outside the workspace")
	require.Len(t, fx.agentPrompts, 3)
}

func TestSkillReadRetainsCurrentSessionValidation(t *testing.T) {
	for _, state := range []string{"disabled", "version_changed", "not_ready", "invalid_digest"} {
		t.Run(state, func(t *testing.T) {
			fx := newInstallFixture(t)
			fx.configRepo.entity.Config.SkillPreparation = "session"
			fx.svc.sessions = &preparationSessions{SessionRepository: fx.svc.sessions, tenant: 7}
			row, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
			require.NoError(t, err)
			row.Status = types.SkillStatusReady
			row.BundleSHA256 = strings.Repeat("a", 64)
			selected := *row
			switch state {
			case "disabled":
				require.NoError(t, fx.skillRepo.UpdateSkillAdminState(
					context.Background(), "cfg-1", row.ID, false, row.Envs,
				))
			case "version_changed":
				row.BundleSHA256 = strings.Repeat("b", 64)
			case "not_ready":
				row.Status = types.SkillStatusInstalling
			case "invalid_digest":
				row.BundleSHA256 = ""
				selected.BundleSHA256 = ""
			}
			require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), row))
			mgr := &preparationSandbox{installSandboxManager: fx.sandboxMgr}
			for _, execute := range []bool{false, true} {
				require.Error(t, fx.svc.prepareSessionSkill(context.Background(), mgr, 7, "user-session", "cfg-1", &selected, execute))
			}
			require.Zero(t, mgr.shellCalls)
			require.Empty(t, fx.agentPrompts)
		})
	}
}
func TestSessionPreparationFailureDoesNotRecordCompletion(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillPreparation = "session"
	fx.svc.sessions = &preparationSessions{SessionRepository: fx.svc.sessions, tenant: 7}
	row, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	row.Status = types.SkillStatusReady
	row.BundleRef = "storage://absent.zip"
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), row))
	mgr := &preparationSandbox{installSandboxManager: fx.sandboxMgr, digest: row.BundleSHA256}
	require.Error(t, fx.svc.prepareSessionSkill(context.Background(), mgr, 7, "user-session", "cfg-1", row, true))
	require.Empty(t, mgr.marker)
	require.Empty(t, fx.agentPrompts)
	row.Enabled = false
	require.NoError(t, fx.skillRepo.UpdateSkillAdminState(
		context.Background(), "cfg-1", row.ID, false, row.Envs,
	))
	require.ErrorContains(t, fx.svc.prepareSessionSkill(context.Background(), mgr, 7, "user-session", "cfg-1", row, true), "disabled")
}

func TestSessionPreparationRefusesWhenWorkspaceScriptsAreDisabled(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillPreparation = "session"
	fx.svc.sandboxPolicy = stubWorkspaceSandboxPolicy{disabled: true}
	sessions := &preparationSessions{tenant: 7}
	fx.svc.sessions = sessions
	messages := &preparationMessages{}
	fx.svc.messages = messages
	mgr := &preparationSandbox{installSandboxManager: fx.sandboxMgr}

	err := fx.svc.prepareSessionSkill(
		context.Background(), mgr, 7, "user-session", "cfg-1",
		&types.TenantSkillEntity{ID: "sk-1"}, true,
	)

	require.ErrorContains(t, err, "disabled")
	require.Zero(t, mgr.shellCalls)
	require.Empty(t, sessions.created)
	require.Empty(t, messages.created)
}
