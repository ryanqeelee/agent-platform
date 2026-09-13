package service

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	errAgentBoom  = errors.New("agent boom")
	errUpdateBoom = errors.New("update boom")
)

func TestRunInstallHappyPathSwitchesPointerLast(t *testing.T) {
	fx := newInstallFixture(t)

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.NoError(t, err)
	require.Equal(t, []string{
		"create-session", "prepare-skill-dir", "seed-files", "agent-execute",
		"chmod", "verify-structure", "verify-python", "write-manifest",
		"cleanup-workspace", "create-snapshot",
		"switch-pointer", "mark-stale", "destroy-sandbox",
	}, fx.events, "the pointer must move only after the snapshot exists")

	cfg := fx.configRepo.saved.Config
	require.Equal(t, "snap-1", cfg.SkillImage.SnapshotID)
	require.Equal(t, 1, cfg.SkillImage.Generation)
	require.Equal(t, "base-template", cfg.SkillImage.BaseTemplateID,
		"the first install must remember the template the image was grown from")
	require.Equal(t, fx.fingerprint, cfg.SkillImage.OwnerFingerprint)
	require.Equal(t, 1, fx.configRepo.updates,
		"the pointer switch must be exactly one config write")
	require.Empty(t, fx.deletedSnapshots,
		"a successful install deletes nothing; the previous image stays reachable")
	require.False(t, fx.loadCheckRanAsRoot,
		"script verification must run as the ordinary sandbox user, not install-mode root")
	require.Empty(t, fx.sessionCalls, "platform installation must not create a business session")

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status)
}

func TestRunInstallSucceedsOnDockerConfig(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.SandboxType = "docker"
	fx.configRepo.entity.Config.SandboxType = "docker"
	fx.configRepo.entity.Config.E2B = nil
	fx.configRepo.entity.Config.Docker = &types.DockerSandboxConfig{
		Image: "weknora/sandbox:base",
		Host:  "unix:///var/run/docker.sock",
	}
	fx.fingerprint = sandbox.SkillImageFingerprint("docker", "", "unix:///var/run/docker.sock")

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.NoError(t, err)
	require.Contains(t, fx.events, "create-snapshot")
	require.Equal(t, "snap-1", fx.configRepo.saved.Config.SkillImage.SnapshotID)
	require.Equal(t, "weknora/sandbox:base", fx.configRepo.saved.Config.SkillImage.BaseTemplateID)
	require.Equal(t, fx.fingerprint, fx.configRepo.saved.Config.SkillImage.OwnerFingerprint)
}

func TestSkillSnapshotBuildNameIncludesFullPlatformConfig(t *testing.T) {
	a := skillSnapshotBuildName("aaaaaaaa-bbbb-cccc-dddd-eeeeffff0001", 1, "11111111-2222-3333-4444-555555555555")
	b := skillSnapshotBuildName("aaaaaaaa-bbbb-cccc-dddd-eeeeffff0002", 1, "11111111-2222-3333-4444-555555555555")
	c := skillSnapshotBuildName("aaaaaaaa-bbbb-cccc-dddd-eeeeffff0001", 1, "aaaaaaaa-bbbb-cccc-dddd-eeeeffff0001")
	require.Equal(t, "weknora-sk-aaaaaaaabbbbccccddddeeeeffff0001-g1-11111111", a)
	require.NotEqual(t, a, b, "two configs must not share a tag")
	require.NotEqual(t, a, c, "two builds of the same generation must not share a tag")
}

func TestNextSnapshotGenerationSkipsAbandonedLedgerRows(t *testing.T) {
	require.Equal(t, 1, nextSnapshotGeneration(0, nil))
	require.Equal(t, 4, nextSnapshotGeneration(2, []*types.TenantSkillSnapshotEntity{
		{Generation: 3, State: types.SkillSnapshotStateBuilding},
		{Generation: 1, State: types.SkillSnapshotStateActive},
	}))
}

// TestRunInstallIssuesExactlyTheseCommands pins the order, not just the set.
// Ownership and permissions are normalised BEFORE verification on purpose: the
// agent creates the tree as root, so a restrictive root umask would leave the
// .venv interpreter unreadable and fail a perfectly good install in the
// non-root verification pass, which must exercise the same permissions the
// snapshot will carry.
func TestRunInstallIssuesExactlyTheseCommands(t *testing.T) {
	fx := newInstallFixture(t)

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.Equal(t, []string{
		installPrepareCommand,
		installToolsProbeCommand(),
		seedExtractCommand(installSkillDir),
		"chmod -R 555 " + installSkillDir + " && chown -R root:root " + installSkillDir,
		skillTreeVerifyCommand(installSkillDir, []string{"scripts/extract.py"}),
		installPythonVerifyCommand,
		cleanImageScratchCommand(),
	}, fx.commands)
}

func TestRunInstallNormalisesPermissionsBeforeVerifying(t *testing.T) {
	fx := newInstallFixture(t)

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.Less(t, indexOfEvent(fx.events, "chmod"), indexOfEvent(fx.events, "verify-python"),
		"the non-root verification pass must execute the permissions that get snapshotted")
}

func TestRunInstallWipesThePreviousTreeBeforeSeeding(t *testing.T) {
	fx := newInstallFixture(t)

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.Equal(t, installPrepareCommand, fx.commands[0])
	require.Less(t, indexOfEvent(fx.events, "prepare-skill-dir"), indexOfEvent(fx.events, "seed-files"),
		"a file dropped between two versions must not survive in the image")
	require.Equal(t, []string{
		sandbox.SkillsManifestPath,
		installSkillDir + "/SKILL.md",
		installSkillDir + "/scripts/extract.py",
	}, fx.sandboxMgr.sortedWrites())
}

func TestRunInstallReportsTransportFailureCause(t *testing.T) {
	fx := newInstallFixture(t)
	// Scoped to the command whose failure this test is about. An unscoped
	// result failed the very first install command instead, so the structural
	// verification this names was never reached.
	fx.execResultCommand = skillTreeVerifyCommand(installSkillDir, []string{"scripts/extract.py"})
	fx.execResult = &sandbox.ExecuteResult{
		ExitCode: -1,
		Killed:   true,
		Error:    "context deadline exceeded",
	}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "skill tree verification failed",
		"this is the structural verification's own failure path")
	require.ErrorContains(t, err, "context deadline exceeded")
	require.ErrorContains(t, err, "killed")

	skill, _ := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.Contains(t, skill.Error, "context deadline exceeded",
		"the admin's only diagnostic is this row")
}

// The tree check's exit 1 is a protocol, not a crash: the findings travel as
// stderr lines and are the whole message. The exec wrapper's generic
// "command failed (...)" text must not shadow them.
func TestRunInstallReportsTheMissingScriptByPath(t *testing.T) {
	fx := newInstallFixture(t)
	fx.execResultCommand = skillTreeVerifyCommand(installSkillDir, []string{"scripts/extract.py"})
	fx.execResult = &sandbox.ExecuteResult{
		ExitCode: 1,
		Stderr:   "script scripts/extract.py is missing after install",
	}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "script scripts/extract.py is missing after install")
	require.NotContains(t, err.Error(), "command failed",
		"protocol exits speak for themselves")
}

// One round trip reports every missing file, not just the first: an install
// that fails anyway may as well fail completely.
func TestRunInstallCollectsEveryMissingFile(t *testing.T) {
	fx := newInstallFixture(t)
	fx.execResultCommand = skillTreeVerifyCommand(installSkillDir, []string{"scripts/extract.py"})
	fx.execResult = &sandbox.ExecuteResult{
		ExitCode: 1,
		Stderr: "SKILL.md is missing after install\n" +
			"script scripts/extract.py is missing after install",
	}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "SKILL.md is missing after install")
	require.ErrorContains(t, err, "script scripts/extract.py is missing after install")
}

// File names come from an uploaded archive; a metacharacter in one must stay
// a literal, so every path reaches the command only through ShellQuote.
func TestSkillTreeVerifyCommandQuotesPaths(t *testing.T) {
	cmd := skillTreeVerifyCommand("/skills/de mo", []string{"scripts/a b.py", "it's.sh"})

	require.Contains(t, cmd, sandbox.ShellQuote("/skills/de mo"))
	require.Contains(t, cmd, sandbox.ShellQuote("scripts/a b.py"))
	require.Contains(t, cmd, sandbox.ShellQuote("it's.sh"))
}

// The tree check costs one round trip however many scripts the bundle carries.
func TestVerifySkillTreeIssuesOneCommandRegardlessOfScriptCount(t *testing.T) {
	fx := newInstallFixture(t)
	files := map[string][]byte{"SKILL.md": []byte(validSkillMD)}
	rels := []string{"run.sh", "scripts/a.py", "scripts/b.py",
		"scripts/c.py", "scripts/d.py", "scripts/e.py"}
	for _, rel := range rels {
		files[rel] = []byte("pass\n")
	}

	require.NoError(t, fx.svc.verifySkillTree(context.Background(),
		fx.sandboxMgr, "sess-1", installSkillDir, &SkillBundle{Name: "pdf-tools", Files: files}))

	require.Len(t, fx.commands, 1,
		"one command for six scripts, not one command per script")
	require.Equal(t, skillTreeVerifyCommand(installSkillDir, rels), fx.commands[0])
}

// The retention half of the cleanup runs for real inside the sandbox, so its
// guard is tested against an actual shell: a cache under the budget survives,
// an over-budget one is wiped whole, and the total is reported either way —
// that report is the data the budget constant is tuned from.
func TestCacheBudgetGuardKeepsSmallWipesBig(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small")
	big := filepath.Join(dir, "big")
	require.NoError(t, os.MkdirAll(small, 0o755))
	require.NoError(t, os.MkdirAll(big, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(small, "tiny.whl"), bytes.Repeat([]byte("x"), 1024), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(big, "huge.whl"), bytes.Repeat([]byte("x"), 2<<20), 0o644))

	kept, err := exec.Command("sh", "-c",
		cacheBudgetGuardCommand([]string{small}, 1024)).CombinedOutput()
	require.NoError(t, err, "the guard must always succeed: %s", kept)
	require.DirExists(t, small, "a cache under the budget is kept")
	require.Contains(t, string(kept), "cache total:")

	wiped, err := exec.Command("sh", "-c",
		cacheBudgetGuardCommand([]string{big}, 1024)).CombinedOutput()
	require.NoError(t, err, "the guard must always succeed: %s", wiped)
	require.NoDirExists(t, big, "a cache over the budget is wiped whole")
	require.Contains(t, string(wiped), "wiped")
}

// The retention command is one opaque string to the sandbox; pinning its
// structure here keeps the shell contract from drifting silently — in
// particular that the workspace half still owns the exit code and the
// retention half can only ever keep or lose caches.
func TestCleanImageScratchCommandShape(t *testing.T) {
	cmd := cleanImageScratchCommand()

	require.Contains(t, cmd, "rm -rf /workspace/* /workspace/.[!.]* || true")
	require.Contains(t, cmd, "mkdir -p")
	require.Contains(t, cmd, "status=$?")
	require.Contains(t, cmd, "exit $status")
	require.Contains(t, cmd, "uv cache prune")
	require.Contains(t, cmd, "npm cache verify")
	require.Contains(t, cmd, "pnpm store prune")
	require.Contains(t, cmd, "du -skc")
	require.Contains(t, cmd, fmt.Sprintf("%d", skillCacheBudgetMB*1024))
	require.Contains(t, cmd, "/root/.cache/uv")
	require.Contains(t, cmd, "/home/"+sandbox.DefaultSandboxExecUser+"/.npm")

	// And it must parse: the command is one opaque string to the sandbox, so a
	// quoting mistake would only surface as a runtime failure there.
	require.NoError(t, exec.Command("sh", "-n", "-c", cmd).Run(),
		"the emitted command must parse under /bin/sh")
}

// The workspace half of the cleanup keeps its old failure semantics across the
// merge: if the session account's input/output directories cannot be restored,
// every session booting from the snapshot lands on a bare /workspace, and that
// fails the install.
func TestRunInstallFailsWhenTheWorkspaceRestoreFails(t *testing.T) {
	fx := newInstallFixture(t)
	fx.execResultCommand = cleanImageScratchCommand()
	fx.execResult = &sandbox.ExecuteResult{
		ExitCode: 1,
		Stderr:   "chown: cannot access '/workspace/input': No such file or directory",
	}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "clean image scratch")
}

// The probe's stdout is the prompt's toolchain section; the parse keeps
// exactly the `name=path` lines and drops everything else rather than
// failing the install over noise.
func TestParseToolProbeOutput(t *testing.T) {
	tools := parseToolProbeOutput(
		"uv=/root/.local/bin/uv\n" +
			"not a tool line\n" +
			"npm=\n" + // cut leaves an empty path: dropped
			"python3=/usr/bin/python3\n" +
			"\n")
	require.Equal(t, map[string]string{
		"uv":      "/root/.local/bin/uv",
		"python3": "/usr/bin/python3",
	}, tools)
}

// The prompt section must name the present tools with their paths and group
// the missing ones, so the agent neither re-discovers nor gambles on PATH.
func TestFormatToolchainSection(t *testing.T) {
	section := formatToolchainSection(map[string]string{
		"uv":      "/root/.local/bin/uv",
		"python3": "/usr/bin/python3",
	})
	require.Contains(t, section, "- uv: /root/.local/bin/uv")
	require.Contains(t, section, "- python3: /usr/bin/python3")
	require.Contains(t, section, "not found: npm, pnpm, pip3, pip, node")

	require.Contains(t, formatToolchainSection(nil),
		"locate tools with `command -v <tool>`")
}

func TestRunInstallReportsVerificationFailureCause(t *testing.T) {
	fx := newInstallFixture(t)
	fx.loadCheckResult = &sandbox.ExecuteResult{ExitCode: -1, Killed: true, Error: "sandbox unreachable"}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "sandbox unreachable")
	require.ErrorContains(t, err, "killed")
}

func TestRunInstallSupersedesThePreviousLedgerRowWithoutDeletingIt(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillImage = &types.SkillImageConfig{
		SnapshotID: "snap-old", Generation: 3,
		BaseTemplateID: "base-template", OwnerFingerprint: fx.fingerprint,
	}
	require.NoError(t, fx.skillRepo.CreateSnapshotRow(context.Background(),
		&types.TenantSkillSnapshotEntity{
			ID: "row-old", SandboxConfigID: "cfg-1", SkillID: "sk-0",
			SnapshotID: "snap-old", Generation: 3,
			Trigger: types.SkillSnapshotTriggerInstall,
			State:   types.SkillSnapshotStateActive,
		}))

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	rows, err := fx.skillRepo.ListSnapshotsByConfig(context.Background(), "cfg-1")
	require.NoError(t, err)
	states := map[string]string{}
	for _, row := range rows {
		states[row.SnapshotID] = row.State
	}
	require.Equal(t, types.SkillSnapshotStateSuperseded, states["snap-old"])
	require.Equal(t, types.SkillSnapshotStateActive, states["snap-1"])
	require.Empty(t, fx.deletedSnapshots,
		"the ledger still names snap-old; deleting it would dangle the row")
	require.Equal(t, 4, fx.configRepo.saved.Config.SkillImage.Generation)
	require.Equal(t, "base-template", fx.configRepo.saved.Config.SkillImage.BaseTemplateID,
		"the base template is recorded once and never re-derived from a snapshot")
}

func TestSwitchImagePointerKeepsAConcurrentNameEdit(t *testing.T) {
	fx := newInstallFixture(t)
	// Config edits are serialised by the config service's own cordon, not by
	// the skill image lock, so a rename can land while this run is still
	// driving the agent. The pointer switch must re-read and keep that name.
	fx.configRepo.editAfterFirstRead = func(e *types.TenantSandboxConfigEntity) {
		e.Name = "renamed"
	}

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	saved := fx.configRepo.saved
	require.Equal(t, "renamed", saved.Name, "the pointer switch must not revert a config edit")
	require.Equal(t, fx.fingerprint, saved.Config.SkillImage.OwnerFingerprint,
		"the snapshot was built under the original credentials")
}

func TestSwitchImagePointerAbandonsSnapshotWhenCredentialsRotate(t *testing.T) {
	fx := newInstallFixture(t)
	// Rotating the API key mid-install does not move the already-created
	// snapshot to the new account. Stamping the new fingerprint onto that ID
	// would make every session trust a snapshot the live key cannot resolve.
	fx.configRepo.editAfterFirstRead = func(e *types.TenantSandboxConfigEntity) {
		e.Config.E2B.APIKey = "key-2"
	}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "credentials changed")
	require.Nil(t, fx.configRepo.saved, "an unresolvable pointer must never be persisted")
	require.Contains(t, fx.deletedSnapshots, "snap-1")

	skill, _ := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.Equal(t, types.SkillStatusFailed, skill.Status)
}

func TestRunInstallAbortsWhenTheSkillRowWasRemoved(t *testing.T) {
	fx := newInstallFixture(t)
	require.NoError(t, fx.skillRepo.DeleteSkill(context.Background(), "cfg-1", "sk-1"))

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.NotContains(t, fx.events, "create-snapshot",
		"a queued install must not bake a skill whose row a remove already deleted")
	require.Nil(t, fx.configRepo.saved)
	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Nil(t, skill)
}

// declareEnvFile presets the file the installer agent is asked to write, so
// the fixture exercises the same air gap production uses: bytes in the
// sandbox, parsing and storage server-side.
func (f *installFixture) declareEnvFile(body string) {
	f.t.Helper()
	if f.sandboxMgr.files == nil {
		f.sandboxMgr.files = map[string][]byte{}
	}
	f.sandboxMgr.files[sandbox.SkillRequirementsPath(f.bundle.Name)] = []byte(body)
}

// readsTavilyKey makes the bundle actually mention the variable, which is what
// the bundle-match layer looks for.
func (f *installFixture) readsTavilyKey() {
	f.bundle.Files["scripts/extract.py"] = []byte("import os\nos.environ[\"TAVILY_API_KEY\"]\n")
}

func TestRunInstallRecordsTheDeclaredEnvVars(t *testing.T) {
	fx := newInstallFixture(t)
	fx.readsTavilyKey()
	fx.declareEnvFile(`{"env":[
		{"name":"TAVILY_API_KEY","description":"search key","required":true,"value":"tvly-invented"},
		{"name":"OPENAI_API_KEY","required":true}
	]}`)

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status)
	require.Equal(t, types.SkillEnvVars{
		{Name: "TAVILY_API_KEY", Description: "search key", Required: true},
	}, skill.Envs,
		"the hallucinated name is dropped and no agent-invented value is ever stored")
}

func TestRunInstallSucceedsWithoutAnEnvDeclaration(t *testing.T) {
	fx := newInstallFixture(t)

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status,
		"getting the skill into the image is the goal; a missing declaration is not a failure")
	require.Empty(t, skill.Envs)
}

func TestRunInstallSucceedsWhenTheEnvDeclarationIsUnparseable(t *testing.T) {
	fx := newInstallFixture(t)
	fx.declareEnvFile("I installed the skill, boss!")

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status)
	require.Empty(t, skill.Envs)
}

func TestRunInstallSucceedsWhenEveryDeclaredEnvVarIsRejected(t *testing.T) {
	fx := newInstallFixture(t)
	fx.declareEnvFile(`{"env":[{"name":"OPENAI_API_KEY"},{"name":"PATH"}]}`)

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status)
	require.Empty(t, skill.Envs)
}

// Re-installing must not destroy a credential an admin typed in, and must not
// keep asking for a variable the new version no longer reads.
func TestRunInstallReinstallKeepsTheAdminValueAndDropsStaleVars(t *testing.T) {
	fx := newInstallFixture(t)
	fx.readsTavilyKey()
	require.NoError(t, fx.skillRepo.UpdateSkillEnvs(context.Background(), "cfg-1", "sk-1",
		types.SkillEnvVars{
			{Name: "TAVILY_API_KEY", Description: "old text", Value: "tvly-typed-by-admin"},
			{Name: "LEGACY_TOKEN", Required: true, Value: "stale"},
		}))
	fx.declareEnvFile(`{"env":[{"name":"TAVILY_API_KEY","description":"new text","required":true}]}`)

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillEnvVars{{
		Name: "TAVILY_API_KEY", Description: "new text", Required: true,
		Value: "tvly-typed-by-admin",
	}}, skill.Envs)
}

func TestRunInstallReinstallOnlyClearsEnvsForAnExplicitEmptyDeclaration(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		writeFile bool
		wantEnvs  types.SkillEnvVars
	}{
		{
			name:     "missing declaration preserves stored value",
			wantEnvs: storedAdminEnv(),
		},
		{
			name:      "unparseable declaration preserves stored value",
			body:      "I installed the skill, boss!",
			writeFile: true,
			wantEnvs:  storedAdminEnv(),
		},
		{
			name:      "fully rejected declaration preserves stored value",
			body:      `{"env":[{"name":"tavily_api_key"},{"name":"PATH"}]}`,
			writeFile: true,
			wantEnvs:  storedAdminEnv(),
		},
		{
			name:      "explicit empty declaration clears stored value",
			body:      `{"env":[]}`,
			writeFile: true,
			wantEnvs:  nil,
		},
		{
			name:      "missing env field preserves stored value",
			body:      `{}`,
			writeFile: true,
			wantEnvs:  storedAdminEnv(),
		},
		{
			name:      "null env field preserves stored value",
			body:      `{"env":null}`,
			writeFile: true,
			wantEnvs:  storedAdminEnv(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newInstallFixture(t)
			require.NoError(t, fx.skillRepo.UpdateSkillEnvs(
				context.Background(), "cfg-1", "sk-1", storedAdminEnv()))
			if tc.writeFile {
				fx.declareEnvFile(tc.body)
			}

			require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

			skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
			require.NoError(t, err)
			require.Equal(t, types.SkillStatusReady, skill.Status)
			require.Equal(t, tc.wantEnvs, skill.Envs)
		})
	}
}

func storedAdminEnv() types.SkillEnvVars {
	return types.SkillEnvVars{{
		Name: "TAVILY_API_KEY", Description: "old text", Required: true,
		Value: "tvly-typed-by-admin",
	}}
}

func TestBuildInstallPromptAsksForADeclarationWithoutValues(t *testing.T) {
	fx := newInstallFixture(t)

	prompt := buildInstallPrompt(installSkillDir, fx.bundle, map[string]string{
		"uv": "/root/.local/bin/uv", "python3": "/usr/bin/python3",
	})

	require.Contains(t, prompt, sandbox.SkillRequirementsPath(fx.bundle.Name))
	require.Contains(t, prompt, ".weknora/requirements.json")
	require.Contains(t, prompt, `{"env":[]}`)
	require.Contains(t, prompt, "Never write any value",
		"a value the model invents would be stored as the workspace credential")
	require.Contains(t, prompt, "WEKNORA_API_KEY",
		"the installer must be told credential names are declarable, or it writes {\"env\":[]}")
	require.Contains(t, prompt, "On-demand / optional extras MUST be installed now")
	require.Contains(t, prompt, "uv venv --seed")
	require.Contains(t, prompt, "install_deps.py")
	require.Contains(t, prompt, "write_skill_file",
		"a heredoc truncates at the command-length cap; the file tools are the writer")
	require.Contains(t, prompt, "write_sandbox_file only writes /workspace",
		"the installer must be told why the workspace writer cannot help it")
	require.Contains(t, prompt, "- uv: /root/.local/bin/uv",
		"the prompt hands over absolute paths instead of a PATH gamble")
}

// shell_exec used to default to /workspace, so the installer opened command
// after command with `cd <skill-dir> &&` — the one spelling guaranteed to land
// somewhere useful. The default is now the skill directory, and the prompt has
// to say so, because a model that is not told keeps paying for the prefix.
func TestBuildInstallPromptSaysCommandsAlreadyStartInTheSkillDirectory(t *testing.T) {
	fx := newInstallFixture(t)

	prompt := buildInstallPrompt(installSkillDir, fx.bundle, nil)

	require.Contains(t, prompt, "shell_exec already starts every command in "+installSkillDir)
	require.Contains(t, prompt, "do NOT prefix")
	require.Contains(t, prompt, "cd <skill-dir> &&")
}

// Import resolution is the agent's job because it is nobody else's: the server
// parses files without executing them, so it never learns whether an import
// would have worked. The prompt has to say so, or the one party holding a real
// interpreter reasons about imports instead of running them.
func TestBuildInstallPromptDemandsImportsBeProvenByRunningThem(t *testing.T) {
	fx := newInstallFixture(t)

	prompt := buildInstallPrompt(installSkillDir, fx.bundle, nil)

	require.Contains(t, prompt, "PROVE the skill's imports resolve")
	require.Contains(t, prompt, "Do not reason about it")
	require.Contains(t, prompt, installSkillDir+"/.venv/bin/python -c 'import x'")
	require.Contains(t, prompt, "never judges an import",
		"the agent must not expect the server to catch an unresolved import")
	require.Contains(t, prompt, "edit_skill_file",
		"a shipped module Python cannot find is fixed in the script, not with pip")
}

func TestBuildInstallPromptNamesOnDemandInstallerInTheArchive(t *testing.T) {
	fx := newInstallFixture(t)
	fx.bundle.Files["scripts/install_deps.py"] = []byte("print(1)\n")

	prompt := buildInstallPrompt(installSkillDir, fx.bundle, map[string]string{
		"uv": "/root/.local/bin/uv", "python3": "/usr/bin/python3",
	})

	require.Contains(t, prompt, "This archive ships on-demand installer(s)")
	require.Contains(t, prompt, "scripts/install_deps.py")
}

func TestBuildInstallPromptMentionsRepairedFrontmatter(t *testing.T) {
	fx := newInstallFixture(t)
	fx.bundle.FrontmatterRepaired = true

	prompt := buildInstallPrompt(installSkillDir, fx.bundle, map[string]string{
		"uv": "/root/.local/bin/uv", "python3": "/usr/bin/python3",
	})

	require.Contains(t, prompt, "YAML frontmatter was automatically repaired")
	require.Contains(t, prompt, "Mention this in your summary")
}

func TestInstallSkillRepoNormalizesStoredUserEnvPrincipal(t *testing.T) {
	repo := &installSkillRepo{}
	row := &types.TenantUserEnvVar{
		TenantID:      7,
		PrincipalType: " web_user ", PrincipalID: " user-1 ",
		SandboxConfigID: "cfg-1", SkillID: "sk-1", Name: "API_KEY", Value: "secret",
	}

	require.NoError(t, repo.UpsertUserEnvVar(context.Background(), row))

	got, err := repo.ListUserEnvVars(
		context.Background(), 7,
		types.Principal{Type: types.PrincipalWebUser, ID: "user-1"}, "cfg-1", "sk-1",
	)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, types.PrincipalWebUser, got[0].PrincipalType)
	require.Equal(t, "user-1", got[0].PrincipalID)
}

func TestWriteReadySkillStateDoesNotStampANewerBundle(t *testing.T) {
	fx := newInstallFixture(t)
	newer := strings.Repeat("b", 64)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: fx.bundle.Name, BundleSHA256: newer,
		Status: types.SkillStatusInstalling,
	}))

	require.NoError(t, fx.svc.writeReadySkillState(
		context.Background(), "cfg-1", "sk-1", "run-1", "snap-stale", fx.bundle))

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusInstalling, skill.Status)
	require.Equal(t, newer, skill.BundleSHA256)
	require.Empty(t, skill.InstalledSnapshotID,
		"this run's snapshot must not be attributed to a newer upload")
}

func TestFailSkillDoesNotStampANewerBundle(t *testing.T) {
	fx := newInstallFixture(t)
	newer := strings.Repeat("b", 64)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: fx.bundle.Name, BundleSHA256: newer,
		Status: types.SkillStatusInstalling,
	}))

	fx.svc.failSkill(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle, errors.New("old run died"))

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusInstalling, skill.Status)
	require.Equal(t, newer, skill.BundleSHA256)
	require.Empty(t, skill.Error)
}

func TestInstallSkillRecoversFromNameConflict(t *testing.T) {
	fx := newInstallFixture(t)
	fx.skillRepo.getByNameMisses = 1
	fx.skillRepo.createErr = errors.New("UNIQUE constraint failed: platform_skills.sandbox_config_id")
	archive := zipBundle(t, map[string]string{"SKILL.md": validSkillMD})

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id,
		"the upload that lost the unique index must reuse the row that won")
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusInstalling, skill.Status)
}

func TestInstallSkillRefusesWhenBundleCannotBeStored(t *testing.T) {
	fx := newInstallFixture(t)
	fx.saveErr = errors.New("object store down")
	archive := zipBundle(t, map[string]string{"SKILL.md": validSkillMD})

	_, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.Error(t, err)
	require.ErrorContains(t, err, "store bundle")
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusFailed, skill.Status,
		"a skill whose archive never landed must not sit at installing")
	require.NotContains(t, fx.events, "create-session")
}

func TestInstallSkillDoesNotAttachAStaleArchiveAfterANewerRunTakesTheRow(t *testing.T) {
	fx := newInstallFixture(t)
	archiveA := zipBundle(t, map[string]string{
		"SKILL.md": strings.Replace(validSkillMD, "version: 1.0.0", "version: 1.0.1", 1),
	})
	bundleB := &SkillBundle{
		Name: fx.bundle.Name, Version: "2.0.0", Description: "newer bundle",
		SHA256: strings.Repeat("b", 64),
	}
	catalogB := &types.TenantSkillCatalogEntity{
		ID: "cat-b", Name: bundleB.Name, Version: bundleB.Version,
		BundleRef: "storage://bundle-b.zip", BundleSHA256: bundleB.SHA256,
	}
	newerTranscript := types.JSON(`[{"role":"assistant","content":"run B owns this row"}]`)

	fx.beforeSaveBundle = func() {
		fx.beforeSaveBundle = nil
		ctx := context.Background()
		row, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
		require.NoError(t, err)
		row.Version = bundleB.Version
		row.Description = bundleB.Description
		row.BundleSHA256 = bundleB.SHA256
		require.NoError(t, fx.skillRepo.UpdateSkill(ctx, row))
		require.NoError(t, fx.skillRepo.BeginSkillRun(
			ctx, "cfg-1", "sk-1", "run-b", types.SkillStatusInstalling, fx.now(),
		))
		require.NoError(t, fx.skillRepo.CreateCatalog(ctx, catalogB))
		require.NoError(t, fx.svc.pointInstallAtCatalog(ctx, row, "run-b", catalogB))
		matched, err := fx.skillRepo.UpdateInstallTranscript(
			ctx, "cfg-1", "sk-1", "run-b", newerTranscript,
		)
		require.NoError(t, err)
		require.True(t, matched)
	}

	_, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archiveA)

	require.Error(t, err)
	require.ErrorContains(t, err, "superseded by a newer operation")
	row, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, "run-b", row.InstallRunID)
	require.Equal(t, catalogB.ID, row.CatalogID)
	require.Equal(t, bundleB.SHA256, row.BundleSHA256)
	require.Equal(t, types.SkillStatusInstalling, row.Status)
	require.JSONEq(t, string(newerTranscript), string(row.InstallTranscript))
	require.Empty(t, row.Error)
	require.Empty(t, fx.sessionCalls)
}

func TestInstallSkillSkipsWhenReadyWithTheSameArchive(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	fx.seedReadySkillWithSHA(bundle.SHA256, "snap-live")

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id)
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusReady, skill.Status,
		"a ready skill whose archive did not change must not be flipped to installing")
	require.Empty(t, fx.sessionCalls, "the same bytes must not boot a billed sandbox")
	require.NotContains(t, fx.events, "create-snapshot")
	require.Nil(t, fx.configRepo.saved, "the image pointer must stay where it is")
	require.Equal(t, 1, fx.savedBundles,
		"a no-op re-upload must not mint a second catalog object")
	require.Empty(t, skill.BundleRef,
		"the install row must not own a zip; readers follow CatalogID")
	require.NotEmpty(t, skill.CatalogID,
		"a skip must still attach the install to the workspace catalog")
}

func TestInstallSkillRetriesAFailedSkillWithTheSameArchive(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: bundle.Name, BundleSHA256: bundle.SHA256,
		Status: types.SkillStatusFailed, Error: "previous run died",
	}))

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id)
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusInstalling, skill.Status,
		"a failed skill is a retry even when the archive digest is unchanged")
}

// Most failed installs fail for a reason the archive cannot fix, so the retry
// runs the bytes already stored rather than asking for them again.
func TestReinstallSkillRerunsTheStoredArchive(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	fx.storedBundles = map[string][]byte{"storage://bundle.zip": archive}
	require.NoError(t, fx.skillRepo.CreateCatalog(context.Background(), &types.TenantSkillCatalogEntity{ID: "cat-retry", Name: bundle.Name, BundleRef: "storage://bundle.zip", BundleSHA256: bundle.SHA256}))
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: bundle.Name, BundleSHA256: bundle.SHA256, CatalogID: "cat-retry",
		Status: types.SkillStatusFailed, Error: "python verification failed",
	}))

	id, err := fx.svc.ReinstallSkill(context.Background(), "cfg-1", "sk-1")

	require.NoError(t, err)
	require.Equal(t, "sk-1", id, "a retry upgrades the same row, it does not fork a second one")
	require.Positive(t, fx.getFileCalls.Load(),
		"the retry must come from the stored archive, not from a re-upload")
}

// A row taken for a retry must stop pointing at the previous platform run.
func TestTakingARowForInstallDropsThePreviousRunsTranscript(t *testing.T) {
	row := &types.TenantSkillEntity{
		ID: "sk-1", Name: "pdf-tools", Status: types.SkillStatusFailed,
		Error:        "python verification failed",
		InstallRunID: "run-old",
	}

	takeSkillRowForInstall(row, &SkillBundle{Name: "pdf-tools", Version: "2.0.0"}, time.Now())

	require.Equal(t, types.SkillStatusInstalling, row.Status)
	require.Empty(t, row.InstallRunID, "the retry has no transcript of its own yet")
	require.Empty(t, row.Error, "the previous failure is not this run's outcome")
}

// Nothing a retry does can recover a skill whose archive is gone, and the
// operator has to be told to upload it rather than to press the button again.
func TestReinstallSkillRefusesWhenTheArchiveIsGone(t *testing.T) {
	fx := newInstallFixture(t)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: "pdf-tools", Status: types.SkillStatusFailed, BundleRef: "",
	}))

	_, err := fx.svc.ReinstallSkill(context.Background(), "cfg-1", "sk-1")

	require.Error(t, err)
	require.ErrorContains(t, err, "not available")
	require.Empty(t, fx.sessionCalls, "a retry that cannot run must not boot a billed sandbox")
}

func TestReinstallSkillRejectsAnUnknownSkill(t *testing.T) {
	fx := newInstallFixture(t)

	_, err := fx.svc.ReinstallSkill(context.Background(), "cfg-1", "nope")

	require.Error(t, err)
	require.ErrorContains(t, err, "skill not found")
	require.Empty(t, fx.sessionCalls)
}

func TestInstallSkillReinstallsWhenTheLiveImageNoLongerCarriesTheSkill(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: bundle.Name, BundleSHA256: bundle.SHA256,
		Status: types.SkillStatusReady,
	}))
	// The pointer was cleared (last-skill removal, or a rebuild from base).
	// The row still says ready, but the files are gone from every new session.

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id)
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusInstalling, skill.Status,
		"a ready row whose files left the image is a repair, not a skip")
}

func TestInstallSkillSkipsAnInFlightInstallOfTheSameArchive(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	// One heartbeat ago: the first run is slow, not gone.
	beat := fx.now().Add(-skillInstallHeartbeatInterval)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: bundle.Name, BundleSHA256: bundle.SHA256,
		Status: types.SkillStatusInstalling, InstallingSince: &beat,
	}))

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id)
	require.Empty(t, fx.sessionCalls,
		"a second upload of the same bytes must not start another billed run")
}

// A run that keeps beating is left alone however long it takes: a single agent
// command may take installCommandTimeout, and an install runs several.
func TestInstallSkillSkipsAnInstallThatIsSlowButStillBeating(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	submitted := fx.now().Add(-3 * installCommandTimeout)
	beat := fx.now().Add(-skillInstallHeartbeatInterval)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: bundle.Name, BundleSHA256: bundle.SHA256,
		Status: types.SkillStatusInstalling, InstallingSince: &beat,
		CreatedAt: submitted,
	}))

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id)
	require.Empty(t, fx.sessionCalls,
		"an install that started long ago but is still beating must not be restarted")
}

func TestInstallSkillRetriesAStaleInFlightInstallOfTheSameArchive(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	// The heartbeat stopped: the process that owned this row is gone.
	stale := fx.now().Add(-skillInstallInFlightSkip - time.Minute)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: bundle.Name, BundleSHA256: bundle.SHA256,
		Status: types.SkillStatusInstalling, InstallingSince: &stale,
		Error: "the previous process is gone",
	}))

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id)
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusInstalling, skill.Status)
	require.NotNil(t, skill.InstallingSince)
	require.Equal(t, fx.now(), *skill.InstallingSince,
		"a dead in-flight row must be allowed to start a new run, not wait for the reaper")
	require.Empty(t, skill.Error)
}

// The ledger records which skill an install snapshotted, not which archive, so
// an installing row must never be answered from the image: the files there may
// belong to the previous bundle of the same skill, and skipping would report a
// success that never happened.
func TestCanSkipInstallNeverAnswersAnInstallingRowFromTheImage(t *testing.T) {
	fx := newInstallFixture(t)
	ctx := context.Background()
	fx.seedReadySkillWithSHA(fx.bundle.SHA256, "snap-live")
	existing, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(t, err)
	stale := fx.now().Add(-skillInstallInFlightSkip - time.Minute)
	existing.Status = types.SkillStatusInstalling
	existing.InstallingSince = &stale

	require.False(t, fx.svc.canSkipInstall(ctx, existing, fx.bundle),
		"a dead install must be retried, not declared done from another bundle's snapshot")
}

// A ready skill is only skipped when the ledger can actually say the files are
// still in the live image. An unreadable ledger must reinstall rather than
// report a success nobody verified.
func TestCanSkipInstallRequiresAReadableLedger(t *testing.T) {
	fx := newInstallFixture(t)
	ctx := context.Background()
	fx.seedReadySkillWithSHA(fx.bundle.SHA256, "snap-live")
	existing, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.True(t, fx.svc.canSkipInstall(ctx, existing, fx.bundle),
		"the same archive of a ready skill still in the image is a no-op")

	fx.skillRepo.listSnapshotsErr = errors.New("ledger unavailable")

	require.False(t, fx.svc.canSkipInstall(ctx, existing, fx.bundle),
		"a skip must be earned by a readable ledger, not assumed")
}

func TestBeatInstallHeartbeatRestampsOnlyAnInstallingRow(t *testing.T) {
	fx := newInstallFixture(t)
	ctx := context.Background()
	stale := fx.now().Add(-time.Hour)
	require.NoError(t, fx.svc.updateSkillFields(ctx, "cfg-1", "sk-1",
		func(e *types.TenantSkillEntity) { e.InstallingSince = &stale }))

	fx.svc.beatInstallHeartbeat(ctx, "cfg-1", "sk-1", "run-1")

	skill, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, fx.now(), *skill.InstallingSince,
		"a live install must keep its liveness timestamp current")

	// A finished run's row is no longer this install's to touch: reviving the
	// timestamp would hide a ready skill from nothing and a newer upload from
	// the reaper.
	require.NoError(t, fx.svc.updateSkillFields(ctx, "cfg-1", "sk-1",
		func(e *types.TenantSkillEntity) {
			e.Status = types.SkillStatusReady
			e.InstallingSince = nil
		}))

	fx.svc.beatInstallHeartbeat(ctx, "cfg-1", "sk-1", "run-1")

	skill, err = fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status)
	require.Nil(t, skill.InstallingSince,
		"a row that left the installing status must not be stamped alive again")
}

// The heartbeat reloads and rewrites the whole row every 30 seconds while an
// install runs, and a declaration or an admin value can be stored in between.
// Writing envs from that stale copy would put the old list back.
func TestBeatInstallHeartbeatLeavesTheDeclarationAlone(t *testing.T) {
	fx := newInstallFixture(t)
	ctx := context.Background()
	stale := fx.now().Add(-time.Hour)
	require.NoError(t, fx.svc.updateSkillFields(ctx, "cfg-1", "sk-1",
		func(e *types.TenantSkillEntity) { e.InstallingSince = &stale }))

	// Stands in for the beat that read the row before the declaration landed.
	before, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.NoError(t, fx.skillRepo.UpdateSkillEnvs(ctx, "cfg-1", "sk-1", storedAdminEnv()))
	require.NoError(t, fx.skillRepo.UpdateSkill(ctx, before))

	skill, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, storedAdminEnv(), skill.Envs,
		"an install-progress write must not carry an old declaration back")
}

func TestStartInstallHeartbeatBeatsUntilStopped(t *testing.T) {
	fx := newInstallFixture(t)
	ctx := context.Background()
	fx.svc.installHeartbeat = time.Millisecond
	stale := fx.now().Add(-time.Hour)
	require.NoError(t, fx.svc.updateSkillFields(ctx, "cfg-1", "sk-1",
		func(e *types.TenantSkillEntity) { e.InstallingSince = &stale }))

	stop := fx.svc.startInstallHeartbeat(ctx, "cfg-1", "sk-1", "run-1")
	require.Eventually(t, func() bool {
		skill, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
		return err == nil && skill.InstallingSince != nil && skill.InstallingSince.Equal(fx.now())
	}, 2*time.Second, time.Millisecond, "the heartbeat must restamp the row while the run works")
	stop()

	// Stopping is what lets the terminal write stand: a beat landing after it
	// would put a serving skill back to installing.
	require.NoError(t, fx.svc.updateSkillFields(ctx, "cfg-1", "sk-1",
		func(e *types.TenantSkillEntity) {
			e.Status = types.SkillStatusReady
			e.InstallingSince = nil
		}))
	time.Sleep(20 * time.Millisecond)
	skill, err := fx.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status)
	require.Nil(t, skill.InstallingSince)
	stop()
}

func TestInstallSkillDoesNotSkipARemovalOfTheSameArchive(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: bundle.Name, BundleSHA256: bundle.SHA256,
		Status: types.SkillStatusRemoving,
	}))

	id, err := fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.NoError(t, err)
	require.Equal(t, "sk-1", id)
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusInstalling, skill.Status,
		"re-uploading during a removal is how the upload cancels it")
}

func TestInstallSkillSkipRefusesToPretendSuccessWhenBundleCannotBeStored(t *testing.T) {
	fx := newInstallFixture(t)
	archive := zipBundle(t, map[string]string{
		"SKILL.md":           validSkillMD,
		"scripts/extract.py": "print('hi')\n",
	})
	bundle, err := ParseSkillBundle(archive)
	require.NoError(t, err)
	fx.seedReadySkillWithSHA(bundle.SHA256, "snap-live")
	fx.saveErr = errors.New("object store down")

	_, err = fx.svc.InstallSkill(context.Background(), "cfg-1", archive)

	require.Error(t, err)
	require.ErrorContains(t, err, "store bundle")
	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusReady, skill.Status,
		"a storage failure on a no-op re-upload must not flip a serving skill to failed")
	require.Empty(t, fx.sessionCalls)
}

func TestRunInstallAbortsWhenTheSameArchiveIsAlreadyServing(t *testing.T) {
	fx := newInstallFixture(t)
	fx.seedReadySkillWithSHA(fx.bundle.SHA256, "snap-live")

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.NotContains(t, fx.events, "create-snapshot",
		"a sibling retry that lost the race to the first run must not grow another snapshot")
	require.Nil(t, fx.configRepo.saved)
	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, skill.Status)
}

func TestRunInstallAbortsWhenANewerBundleOwnsTheRow(t *testing.T) {
	fx := newInstallFixture(t)
	newer := strings.Repeat("b", 64)
	require.NoError(t, fx.skillRepo.UpdateSkill(context.Background(), &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-1",
		Name: fx.bundle.Name, BundleSHA256: newer,
		Status: types.SkillStatusInstalling,
	}))

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.NotContains(t, fx.events, "create-snapshot")
	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, newer, skill.BundleSHA256)
	require.Equal(t, types.SkillStatusInstalling, skill.Status,
		"failing this run must not stamp the newer owner's row")
}

func TestRunInstallRequiresVenvWhenRequirementsExist(t *testing.T) {
	fx := newInstallFixture(t)
	fx.bundle.Files["requirements.txt"] = []byte("pypdf==4.0.0\n")
	fx.depsExitCode = 1

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, ".venv")
	require.NotContains(t, fx.events, "create-snapshot")
	require.Nil(t, fx.configRepo.saved)
}

// Verification used to run one guessed "entry script" with --help. A skill is
// not one entry point: it is every file the tree ships, some run as scripts
// and some imported as packages. The guess left every other file unchecked
// while failing installs over the one it happened to pick.
func TestRunInstallVerifiesEveryScriptOfEveryLanguage(t *testing.T) {
	fx := newInstallFixture(t)
	fx.bundle.Files["scripts/__init__.py"] = []byte("from .helper import run\n")
	fx.bundle.Files["scripts/helper.py"] = []byte("def run():\n    pass\n")
	fx.bundle.Files["bin/render.mjs"] = []byte("export const x = 1;\n")
	fx.bundle.Files["bin/setup.sh"] = []byte("echo hi\n")

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	pythonPass := skillPythonVerifyCommand(installSkillDir, []string{
		"scripts/__init__.py", "scripts/extract.py", "scripts/helper.py",
	}, nil)
	require.Contains(t, fx.commands, pythonPass,
		"every python file must be checked, not the first one in sort order")
	require.Contains(t, fx.commands,
		skillNodeVerifyCommand(installSkillDir, []string{"bin/render.mjs"}, nil))
	require.Contains(t, fx.commands,
		skillShellVerifyCommand(installSkillDir, []string{"bin/setup.sh"}))
}

// The pick that broke a real install: "__" sorts before every lowercase
// letter, so a package marker became the smoke target and was executed as a
// script, which no relative import can survive.
func TestRunInstallDoesNotExecuteAnyScriptToVerifyIt(t *testing.T) {
	fx := newInstallFixture(t)
	fx.bundle.Files["scripts/__init__.py"] = []byte("from .extract import main\n")

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	for _, command := range fx.commands {
		require.NotContains(t, command, "--help",
			"verification must not invoke a skill script; %q does", command)
	}
}

func TestSkillPythonVerifyCommandPrefersTheSkillVenv(t *testing.T) {
	command := skillPythonVerifyCommand("/opt/skills/demo", []string{"scripts/a.py"}, nil)

	require.Contains(t, command, "if [ -x /opt/skills/demo/.venv/bin/python ]; "+
		"then py=/opt/skills/demo/.venv/bin/python; else py=python3; fi",
		"the file must be checked by the interpreter that would run it")
	require.True(t, strings.HasSuffix(command, `"$py" - /opt/skills/demo scripts/a.py`),
		"the verifier reads from stdin, so the paths are its argv")
}

// A skill may ship a file whose name needs quoting; the command is assembled
// by hand, so the shell must never see it as more than one word.
func TestSkillVerifyCommandsQuoteAwkwardPaths(t *testing.T) {
	python := skillPythonVerifyCommand("/opt/skills/demo", []string{"scripts/a b'c.py"}, nil)
	require.Contains(t, python, `'scripts/a b'\''c.py'`)

	shell := skillShellVerifyCommand("/opt/skills/demo", []string{"a b.sh"})
	require.Equal(t,
		`if command -v bash >/dev/null 2>&1; then parser=bash; else parser=sh; fi; `+
			`for f in '/opt/skills/demo/a b.sh'; do "$parser" -n "$f" || exit 1; done`,
		shell)
}

// A skill's shell scripts carry a bash shebang almost exclusively, and
// SkillInterpreterCommand runs them with bash. Checking them with sh — dash on
// Debian — refused installs over array literals, `function f()`, C-style for
// loops and process substitution, none of which dash can parse.
func TestSkillShellVerifyCommandParsesWithBash(t *testing.T) {
	command := skillShellVerifyCommand("/opt/skills/demo", []string{"bin/setup.sh"})

	require.Contains(t, command, "parser=bash")
	require.Contains(t, command, `"$parser" -n "$f"`)
	require.Contains(t, command, "else parser=sh",
		"an image without bash must still get a syntax check")
	require.NotContains(t, command, `sh -n "$f"`,
		"the check must not hard-code the shell that cannot parse these files")
}

// The Python pass is the only one whose verdict depends on what the image
// carries, so it is the only one that can fail an install over a file nothing
// the skill offers ever loads. Those files are named separately.
func TestSkillPythonVerifyCommandSeparatesAuxiliaryFiles(t *testing.T) {
	command := skillPythonVerifyCommand("/opt/skills/demo",
		[]string{"scripts/run.py"}, []string{"tests/test_run.py"})

	require.True(t, strings.HasSuffix(command,
		`- /opt/skills/demo scripts/run.py --optional tests/test_run.py`),
		"got %q", command)
}

// The split follows the conventions the language ecosystems already share. A
// bundled tests/ directory imports pytest and an examples/ script imports
// whatever it illustrates; neither is reachable from anything the skill
// exposes, and neither may decide whether the skill installs.
func TestSkillAuxiliaryScriptFollowsEcosystemConventions(t *testing.T) {
	for _, rel := range []string{
		"tests/test_run.py", "test/helpers.py", "examples/demo.py",
		"scripts/__tests__/a.py", "benchmarks/bench.py", "docs/conf.py",
		"conftest.py", "setup.py", "scripts/extract_test.py",
		"scripts/test_extract.py",
	} {
		require.True(t, skillAuxiliaryScript(rel), "%s must be auxiliary", rel)
	}
	for _, rel := range []string{
		"scripts/extract.py", "scripts/office/validators/base.py",
		"recalc.py", "scripts/latest.py", "scripts/contest.py",
	} {
		require.False(t, skillAuxiliaryScript(rel), "%s is part of what the skill offers", rel)
	}

	entry, auxiliary := splitAuxiliaryScripts([]string{
		"scripts/a.py", "tests/test_a.py", "scripts/b.py",
	})
	require.Equal(t, []string{"scripts/a.py", "scripts/b.py"}, entry)
	require.Equal(t, []string{"tests/test_a.py"}, auxiliary)
}

func TestSkillNodeVerifyCommandChecksDeclaredDependencies(t *testing.T) {
	bundle := &SkillBundle{Files: map[string][]byte{
		"package.json": []byte(`{"dependencies":{"echarts":"^5"},"devDependencies":{"jest":"^29"}}`),
	}}

	require.Equal(t, []string{"echarts"}, nodeDependencyNames(bundle),
		"devDependencies are a build-time concern the image is not asked to carry")

	command := skillNodeVerifyCommand("/opt/skills/demo", []string{"a.js"}, []string{"echarts"})
	require.Contains(t, command, `[ -e /opt/skills/demo/node_modules/"$d" ]`)
	require.Contains(t, command, `node --check "$f"`)
}

func TestSortedScriptPathsIsDeterministic(t *testing.T) {
	bundle := &SkillBundle{Files: map[string][]byte{
		"scripts/z-helper.py": []byte("x"),
		"scripts/a-main.py":   []byte("x"),
		"scripts/mid.js":      []byte("x"),
		"SKILL.md":            []byte("x"),
		"data/table.csv":      []byte("x"),
	}}

	require.Equal(t, []string{"scripts/a-main.py", "scripts/z-helper.py"},
		sortedScriptPaths(bundle, ".py"))
	require.Equal(t, []string{"scripts/a-main.py", "scripts/mid.js", "scripts/z-helper.py"},
		sortedScriptPaths(bundle, allScriptExtensions...),
		"only files the runtime can execute are scripts")
}

type stubWorkspaceSandboxPolicy struct {
	disabled bool
}

func (s stubWorkspaceSandboxPolicy) WorkspaceScriptsDisabled(context.Context, uint64) (bool, error) {
	return s.disabled, nil
}

func TestSwitchImagePointerRefusesAnUnusableFingerprint(t *testing.T) {
	fx := newInstallFixture(t)
	// A config whose provider cannot snapshot produces no fingerprint,
	// and a pointer with an empty fingerprint is discarded at session start.
	fx.configRepo.entity.SandboxType = "docker"
	fx.configRepo.entity.Config.SandboxType = "docker"
	fx.configRepo.entity.Config.E2B = nil
	fx.configRepo.entity.Config.Docker = nil

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "fingerprint")
	require.Nil(t, fx.configRepo.saved, "an ineffective pointer must never be persisted")
	require.NotContains(t, fx.events, "create-snapshot",
		"a config that cannot own a skill image must not spend a billed snapshot")
	require.Empty(t, fx.deletedSnapshots)
}

// TestRunInstallStopsWhenTheLockIsLost also covers the cleanup that has to
// survive the cancellation it compensates for: withConfigLock cancels the
// install context when lock renewal fails, and both the sandbox destroy and
// the terminal "failed" write are provider/DB calls that would fail on that
// context. The fakes below refuse a cancelled context for exactly that reason.
func TestRunInstallStopsWhenTheLockIsLost(t *testing.T) {
	fx := newInstallFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := fx.svc.runInstall(ctx, "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.NotContains(t, fx.events, "create-snapshot",
		"losing the lock means another install may already be writing; do not snapshot")
	require.Nil(t, fx.configRepo.saved)
	require.Empty(t, fx.destroyedSandboxes,
		"the lock was already gone before a sandbox was created")

	skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusFailed, skill.Status,
		"a row stuck at installing tells the admin nothing and blocks the next upload")
	require.NotEmpty(t, skill.Error)
}

// TestRunInstallCleanupSurvivesAnInstallLongerThanTheCleanupBudget covers the
// deadline half of the compensation contract, which the lock-loss test above
// cannot see: every fixture install finishes in microseconds, so a budget
// started at runInstall's entry is still fresh when cleanup runs. A real
// install spends minutes driving an agent whose single commands may take
// installCommandTimeout, so the budget must start when the compensating work
// does. The install below takes four times the (injected) budget.
func TestRunInstallCleanupSurvivesAnInstallLongerThanTheCleanupBudget(t *testing.T) {
	fx := newInstallFixture(t)
	fx.svc.cleanupTimeout = 50 * time.Millisecond
	fx.agentDelay = 200 * time.Millisecond

	start := time.Now()
	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)
	require.Greater(t, time.Since(start), fx.svc.cleanupTimeout,
		"the install must outlast the cleanup budget for this test to mean anything")

	require.NoError(t, err, "a successful install must not fail on the terminal ready write")
	require.Equal(t, []string{"sess-1"}, fx.destroyedSandboxes,
		"a sandbox left running on the provider is billed until its TTL expires")

	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusReady, skill.Status,
		"the row that says the skill is serving must still be written")
	require.Equal(t, "snap-1", skill.InstalledSnapshotID)
}

// The failure path has the same deadline problem: failSkill is the admin's
// only diagnostic and it runs after the whole install has elapsed.
func TestRunInstallFailureIsRecordedAfterALongInstall(t *testing.T) {
	fx := newInstallFixture(t)
	fx.svc.cleanupTimeout = 50 * time.Millisecond
	fx.agentDelay = 200 * time.Millisecond
	fx.agentErr = errAgentBoom

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.Equal(t, []string{"sess-1"}, fx.destroyedSandboxes)

	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	require.Equal(t, types.SkillStatusFailed, skill.Status,
		"a row stuck at installing leaves the cause in a log line nobody reads")
	require.Contains(t, skill.Error, errAgentBoom.Error())
}

func TestInstallerRuntimeConsumesPersistedPlatformAgentConfig(t *testing.T) {
	require.NoError(t, types.LoadBuiltinAgentsConfig(filepath.Join("..", "..", "..", "config")))
	fx := newInstallFixture(t)
	fx.installerRecord = &types.CustomAgent{
		ID: types.BuiltinSkillInstallerID,
		Config: types.CustomAgentConfig{
			ModelID:       "model-agent",
			SystemPrompt:  "install only declared dependencies",
			AllowedTools:  []string{tools.ToolShellExec},
			MaxIterations: 11,
		},
	}

	config, model, err := fx.svc.resolveInstallerRuntime(
		context.Background(), "cfg-1", installSkillDir,
	)
	require.NoError(t, err)
	require.Equal(t, fx.installerRecord.Config.SystemPrompt, config.SystemPrompt)
	require.Equal(t, fx.installerRecord.Config.MaxIterations, config.MaxIterations)
	require.Contains(t, config.AllowedTools, tools.ToolWriteSkillFile)
	require.Contains(t, config.AllowedTools, tools.ToolEditSkillFile)
	require.True(t, config.SkillInstallMode())
	require.Equal(t, installSkillDir, config.SkillInstallDir(),
		"the file tools must be scoped to this install's own skill directory")
	require.Equal(t, "model-agent", model.GetModelID())
}

func TestResetSkillDirRefusesTheSkillsRoot(t *testing.T) {
	fx := newInstallFixture(t)

	err := fx.svc.resetSkillDir(context.Background(), fx.sandboxMgr, "sess-1", sandbox.SkillsImageRoot)

	require.Error(t, err,
		"an empty skill ID collapses to the skills root, and rm -rf there destroys every other skill")
	require.Empty(t, fx.commands, "the command must not be issued at all")
}

func TestRunInstallDoesNotFailASkillThatIsAlreadyServing(t *testing.T) {
	fx := newInstallFixture(t)
	// The pointer has moved: the skill is installed, snapshotted and serving
	// every new session. Only the row that says so is missing.
	fx.skillRepo.updateFailsWhen = func(e *types.TenantSkillEntity) bool {
		return e.Status == types.SkillStatusReady
	}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.NotNil(t, fx.configRepo.saved, "the pointer switch itself succeeded")
	require.Equal(t, 3, fx.skillRepo.readyWriteAttempts,
		"a transient write failure after the point of no return must be retried")

	skill, getErr := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, getErr)
	// R9 leaves the row exactly as the install found it: still "installing",
	// with no error text, because labelling a serving skill failed would send
	// admins to fix something that works. The residual — a row that stays
	// "installing" once all three retries fail — is the stuck-run reaper's to
	// resolve, and this assertion is what will flip when it does.
	require.Equal(t, types.SkillStatusInstalling, skill.Status,
		"a serving skill must not be labelled failed; the ready write is what is missing")
	require.Empty(t, skill.Error)
}

func TestRunInstallKeepsOldImageWhenVerificationFails(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillImage = &types.SkillImageConfig{
		SnapshotID: "snap-old", Generation: 3, OwnerFingerprint: fx.fingerprint,
	}
	fx.loadCheckExitCodes = []int{1}
	fx.loadCheckStderr = "scripts/extract.py has a syntax error on line 1: invalid syntax"

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.NotContains(t, fx.events, "create-snapshot",
		"a failed verification must not produce a snapshot at all")
	require.Nil(t, fx.configRepo.saved,
		"the image pointer must be untouched so the previous image keeps serving")

	skill, _ := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.Equal(t, types.SkillStatusFailed, skill.Status)
	require.NotEmpty(t, skill.Error)
}

// The failure this closes: the installer derives what to install from SKILL.md
// prose, and the gate derives what must resolve from the imports every file
// executes. Those disagree on any skill whose library modules import something
// its documentation never names — the official office toolkit imports
// defusedxml and lxml at module level and mentions neither — so the install
// died with the answer already in hand, and a retry replayed the same prompt to
// the same effect. The gate's own lines are the repair round's brief.
func TestRunInstallHandsVerificationFindingsBackToTheInstaller(t *testing.T) {
	fx := newInstallFixture(t)
	fx.loadCheckExitCodes = []int{skillVerifyRepairableExit, 0}
	fx.loadCheckStderr = "scripts/office/validators/base.py imports defusedxml, " +
		"which is not available in this image\n" +
		"scripts/recalc.py imports openpyxl, which is not available in this image"

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.Len(t, fx.agentPrompts, 2, "a fixable failure must reach the installer again")
	repair := fx.agentPrompts[1]
	require.Contains(t, repair, "imports defusedxml",
		"the repair round is driven by the gate's own findings, not by a second guess")
	require.Contains(t, repair, "imports openpyxl")
	require.Contains(t, repair, installSkillDir+"/.venv",
		"the round has to be told where the packages belong")
	require.Contains(t, repair, "Do NOT edit",
		"a repair must not be allowed to edit the tree into passing")

	require.Equal(t, 2, fx.loadCheckPasses, "the repair has to be verified, not trusted")
	require.Contains(t, fx.events, "create-snapshot")
	require.NotNil(t, fx.configRepo.saved, "a repaired install must reach the image pointer")

	skill, _ := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.Equal(t, types.SkillStatusReady, skill.Status)
	require.Empty(t, skill.Error)
}

// A repair round has to be able to write into .venv again, which the
// verification pass just locked down: the checks run as the ordinary user, so
// the tree is handed over at 555 and root-owned before every one of them.
func TestRunInstallReopensTheTreeBeforeARepairRound(t *testing.T) {
	fx := newInstallFixture(t)
	fx.loadCheckExitCodes = []int{skillVerifyRepairableExit, 0}

	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))

	reopen := indexOfCommandContaining(fx.commands, "chmod -R u+rwX,go+rX "+installSkillDir)
	require.GreaterOrEqual(t, reopen, 0, "the tree must be writable again for the repair")

	firstLock := indexOfCommandContaining(fx.commands, "chmod -R 555 "+installSkillDir)
	require.GreaterOrEqual(t, firstLock, 0)
	require.Greater(t, reopen, firstLock,
		"the tree is reopened after the pass that locked it, not before")

	// And locked again for the pass that checks the repair, so the final image
	// carries the modes every verification ran against.
	require.Greater(t, lastIndexOfCommandContaining(fx.commands, "chmod -R 555 "+installSkillDir),
		reopen, "the repaired tree must be locked down again before it is verified")
}

// Installing a package cannot fix a file that does not parse, and the bundle
// has to change instead. Spending another agent round on it would only delay
// the same failure by minutes.
func TestRunInstallDoesNotRetryAFailureInstallingCannotFix(t *testing.T) {
	fx := newInstallFixture(t)
	fx.loadCheckExitCodes = []int{1}
	fx.loadCheckStderr = "scripts/extract.py has a syntax error on line 1: invalid syntax"

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "syntax error")
	require.Len(t, fx.agentPrompts, 1, "an unfixable finding must not buy another round")
	require.Equal(t, 1, fx.loadCheckPasses)
}

// The loop is bounded. An installer that cannot satisfy the gate in one repair
// is not going to satisfy it in ten, and a skill install must not become an
// open-ended retry against a billed sandbox.
func TestRunInstallStopsAfterOneRepairRound(t *testing.T) {
	fx := newInstallFixture(t)
	fx.loadCheckExitCodes = []int{skillVerifyRepairableExit}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "imports pandas",
		"the failure must name what the gate found, not just that it failed")
	require.Len(t, fx.agentPrompts, skillInstallVerifyRounds)
	require.Equal(t, skillInstallVerifyRounds, fx.loadCheckPasses)
	require.NotContains(t, fx.events, "create-snapshot")
	require.Nil(t, fx.configRepo.saved)
}

func TestRunInstallDeletesTheSnapshotWhenSwitchFails(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.updateErr = errUpdateBoom

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.Contains(t, fx.deletedSnapshots, "snap-1",
		"a snapshot nobody points at is an orphan; it must be cleaned up here")
}

// The same precondition the removal runs: a stored snapshot the live
// credentials cannot resolve would boot the base template, so this install
// would stack onto an image that carries none of the tenant's other skills and
// then make it current.
func TestRunInstallRefusesAnImageThatBelongsToAnotherProviderAccount(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillImage = &types.SkillImageConfig{
		SnapshotID: "snap-old", Generation: 3,
		BaseTemplateID: "base-template", OwnerFingerprint: fx.fingerprint,
	}
	fx.configRepo.entity.Config.E2B.APIKey = "key-2"

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.ErrorContains(t, err, "provider account")
	require.Empty(t, fx.events, "no sandbox may be started on the wrong image")
	require.Nil(t, fx.configRepo.saved,
		"switching to an image built on the base template would drop every other skill")

	skill, _ := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.Equal(t, types.SkillStatusFailed, skill.Status)
}

func TestRunInstallDeletesTheSnapshotWhenTheLedgerCannotRecordIt(t *testing.T) {
	fx := newInstallFixture(t)
	fx.skillRepo.markStateFails = func(state string) bool {
		return state == types.SkillSnapshotStateActive
	}

	err := fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.Contains(t, fx.deletedSnapshots, "snap-1",
		"a snapshot no row names is unreachable and billed; it must not be leaked")
	require.Nil(t, fx.configRepo.saved)
}

func TestRunInstallDeletesTheOrphanSnapshotAfterTheLockIsLost(t *testing.T) {
	fx := newInstallFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fx.cancelDuringSnapshot = cancel

	err := fx.svc.runInstall(ctx, "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Error(t, err)
	require.Nil(t, fx.configRepo.saved, "the pointer switch runs on the dead context and fails")
	require.Contains(t, fx.deletedSnapshots, "snap-1",
		"cleaning up on the context that just died leaves a billed snapshot nobody can reach")
	require.Equal(t, []string{"sess-1"}, fx.destroyedSandboxes)
}

func TestRunInstallAlwaysDestroysTheTransientSandboxWithoutBusinessSession(t *testing.T) {
	fx := newInstallFixture(t)
	fx.agentErr = errAgentBoom

	_ = fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle)

	require.Equal(t, []string{"sess-1"}, fx.destroyedSandboxes,
		"the install sandbox is released, and only that one")
	require.Empty(t, fx.sessionCalls, "platform maintenance must not create business sessions")
}

func TestResolveInstallerModelPrefersTheAgentsOwnModel(t *testing.T) {
	fx := newInstallFixture(t)

	model, err := fx.svc.resolveInstallerModel(context.Background(), &types.CustomAgent{
		ID:     types.BuiltinSkillInstallerID,
		Config: types.CustomAgentConfig{ModelID: "model-agent"},
	})

	require.NoError(t, err)
	require.Equal(t, "model-agent", model.GetModelID(),
		"whoever configured the installer agent chose that model for this job")
}

func TestResolveInstallerModelFallsBackWhenTheAgentModelIsGone(t *testing.T) {
	fx := newInstallFixture(t)
	fx.modelSvc.missing = map[string]bool{"model-gone": true}

	model, err := fx.svc.resolveInstallerModel(context.Background(), &types.CustomAgent{
		ID:     types.BuiltinSkillInstallerID,
		Config: types.CustomAgentConfig{ModelID: "model-gone"},
	})

	require.NoError(t, err)
	require.Equal(t, "model-1", model.GetModelID())
}

func TestResolveInstallerModelFallsBackWhenTheAgentNamesNoModel(t *testing.T) {
	fx := newInstallFixture(t)

	model, err := fx.svc.resolveInstallerModel(context.Background(), &types.CustomAgent{
		ID: types.BuiltinSkillInstallerID,
	})

	require.NoError(t, err)
	require.Equal(t, "model-1", model.GetModelID())
}

func TestRunInstallPublishesRunLocatorBeforeTheAgentRuns(t *testing.T) {
	fx := newInstallFixture(t)

	var atExecute *types.TenantSkillEntity
	fx.beforeExecute = func() {
		skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
		require.NoError(t, err)
		copied := *skill
		atExecute = &copied
	}

	require.NoError(t, fx.svc.runInstall(ctxWithTenant(7), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.NotNil(t, atExecute, "the installer engine never ran")
	require.Equal(t, "run-1", atExecute.InstallRunID)
}

func TestRunInstallPublishesRunLocatorBeforeSeedingFiles(t *testing.T) {
	fx := newInstallFixture(t)

	var atSeed *types.TenantSkillEntity
	fx.beforeSeed = func() {
		skill, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
		require.NoError(t, err)
		copied := *skill
		atSeed = &copied
	}

	require.NoError(t, fx.svc.runInstall(ctxWithTenant(7), "cfg-1", "sk-1", "run-1", fx.bundle))

	require.NotNil(t, atSeed, "files were seeded without a hook")
	require.Equal(t, "run-1", atSeed.InstallRunID)
}

func TestPackSkillTarRoundTrip(t *testing.T) {
	bundle := &SkillBundle{Files: map[string][]byte{
		"SKILL.md":           []byte("name: x"),
		"scripts/extract.py": []byte("print(1)\n"),
	}}
	raw, err := packSkillTar(bundle)
	require.NoError(t, err)

	got := map[string][]byte{}
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		got[hdr.Name] = content
	}
	require.Equal(t, bundle.Files, got)
}

func TestPackSkillTarRejectsEscapingNames(t *testing.T) {
	_, err := packSkillTar(&SkillBundle{Files: map[string][]byte{
		"../etc/passwd": []byte("x"),
	}})
	require.Error(t, err)
}

const installSkillDir = "/opt/weknora/tenant/skills/pdf-tools"

// The install commands are asserted verbatim: an install runs as root with the
// skills root writable, so "the command contained this substring" is not a
// strong enough statement about what actually executes.
const (
	installPrepareCommand = "rm -rf " + installSkillDir +
		" && mkdir -p /opt/weknora/tenant/skills " + installSkillDir +
		" && chown user:user /opt/weknora/tenant/skills " + installSkillDir +
		" && chmod 755 /opt/weknora/tenant/skills " + installSkillDir
)

// installPythonVerifyCommand is built from the same helper the install path
// uses. Pinning the literal text would mean re-encoding the embedded verifier
// by hand on every edit to it, which is a test that fails for the wrong
// reason; the properties worth stating verbatim are asserted separately in
// TestSkillPythonVerifyCommand*.
var installPythonVerifyCommand = skillPythonVerifyCommand(
	installSkillDir, []string{"scripts/extract.py"}, nil,
)

func indexOfEvent(events []string, needle string) int {
	for i, event := range events {
		if event == needle {
			return i
		}
	}
	return -1
}

// indexOfCommandContaining and its Last variant locate one shell command in the
// ordered transcript. A repair round issues the same commands twice, so the
// tests that care about ordering need both ends.
func indexOfCommandContaining(commands []string, needle string) int {
	for i, command := range commands {
		if strings.Contains(command, needle) {
			return i
		}
	}
	return -1
}

func lastIndexOfCommandContaining(commands []string, needle string) int {
	for i := len(commands) - 1; i >= 0; i-- {
		if strings.Contains(commands[i], needle) {
			return i
		}
	}
	return -1
}

// staleMark is one request to mark a config's bound sandboxes stale. The
// tenant is part of it because marking the right config of the wrong workspace
// would rebuild sandboxes that never carried this image.
type staleMark struct {
	tenantID uint64
	configID string
}

type installFixture struct {
	t          *testing.T
	svc        *TenantSkillService
	bundle     *SkillBundle
	configRepo *installConfigRepo
	skillRepo  *installSkillRepo
	sandboxMgr *installSandboxManager
	modelSvc   *installModelService
	// events are the coarse milestones the ordering tests read; commands is
	// the full, ordered shell transcript so a new command can never hide
	// behind an older substring match.
	events      []string
	commands    []string
	fingerprint string
	// loadCheck* drive the per-language script verification pass, which is the
	// last gate before the snapshot. exitCodes is consumed one entry per python
	// pass and its last entry repeats, so a test about a single round writes one
	// value and a test about the repair round writes two. Exit 2 is the
	// checker's "everything I found is a missing dependency", which is what
	// earns another installer round.
	loadCheckExitCodes []int
	loadCheckStdout    string
	loadCheckStderr    string
	loadCheckPasses    int
	loadCheckResult    *sandbox.ExecuteResult
	// depsExitCode fails the declared-dependency check (venv / node_modules).
	depsExitCode int
	// execResult is scoped to execResultCommand: an unscoped stub result
	// applies to the first command issued, which is not the command any of
	// these tests is about.
	execResultCommand  string
	execResult         *sandbox.ExecuteResult
	loadCheckRanAsRoot bool
	agentErr           error
	// agentPrompts is every prompt the installer engine was handed, in order.
	// A repair round is a second entry, and what it says is the whole point of
	// having one.
	agentPrompts []string
	// beforeExecute runs at the moment the engine would start, so a test can
	// observe the state an attaching console would see mid-install.
	beforeExecute func()
	// beforeSeed runs on the first image file write, so a test can prove the
	// transcript locators landed before the minutes-long copy begins.
	beforeSeed func()
	// staleMarks records every InvalidateConfigSandboxes call, so a test can
	// state which config was marked rather than only that something was.
	staleMarks []staleMark
	// invalidateErr fails the marking the way an unreachable binding store
	// would, without failing anything else the run does.
	invalidateErr error
	// rmExitCode fails the removal's directory wipe, the one image step a
	// removal has.
	rmExitCode int
	// cancelDuringRemove models the lock renewal failing mid-run, which is
	// when a real run loses its context.
	cancelDuringRemove context.CancelFunc
	// cancelDuringSnapshot models the lock renewal failing at the one moment
	// a run cannot stop for it: the snapshot already exists, so the pointer
	// switch that follows fails on the dead context and leaves an orphan.
	cancelDuringSnapshot context.CancelFunc
	// cancelDuringConfigRead models the same failure one step earlier, while
	// the run is still deciding what to do.
	cancelDuringConfigRead context.CancelFunc
	// agentDelay and removeDelay make the run take real time, the way a
	// dependency install or a large tree wipe does. They are what let a test
	// outlive the cleanup budget.
	agentDelay         time.Duration
	removeDelay        time.Duration
	deletedSnapshots   []string
	destroyedSandboxes []string
	deletedBundles     []string
	sessionCalls       []string
	// manifest is what the seeded image claims to carry; the sandbox fake
	// serves it back to whoever reads SkillsManifestPath.
	manifest skillImageManifest
	// installerRecord is the platform-owned agent row the installer resolver serves.
	installerRecord *types.CustomAgent
	// saveErr fails bundle storage so InstallSkill cannot accept a skill
	// whose archive will later be unreadable.
	saveErr          error
	beforeSaveBundle func()
	savedBundles     int
	// storedBundles is what GetFile serves back, keyed by the SaveBytes
	// reference, so ListSkillFiles / ReadSkillFile can open a stored archive.
	storedBundles map[string][]byte
	getFileCalls  atomic.Int32
}

func newInstallFixture(t *testing.T) *installFixture {
	t.Helper()

	fx := &installFixture{t: t}
	fx.installerRecord = &types.CustomAgent{
		ID:        types.BuiltinSkillInstallerID,
		TenantID:  platformAgentTenantID,
		IsBuiltin: true,
		Config: types.CustomAgentConfig{
			ModelID:      "model-1",
			AgentMode:    types.AgentModeSmartReasoning,
			AllowedTools: []string{"shell_exec"},
		},
	}
	fx.fingerprint = sandbox.SkillImageFingerprint("e2b", "key-1", "https://e2b.example")
	// The checker's own wording for a missing dependency, so a test reading the
	// repair prompt sees what a real run would put in it.
	fx.loadCheckStderr = "scripts/extract.py imports pandas, " +
		"which is not available in this image"
	fx.bundle = &SkillBundle{
		Name:         "pdf-tools",
		Version:      "1.0.0",
		Description:  "Extract text from PDF files",
		Instructions: "Use scripts/extract.py to pull text out of a PDF.",
		SHA256:       strings.Repeat("a", 64),
		Files: map[string][]byte{
			"SKILL.md":           []byte(validSkillMD),
			"scripts/extract.py": []byte("print('hi')\n"),
		},
	}
	fx.configRepo = &installConfigRepo{fx: fx, entity: &types.TenantSandboxConfigEntity{
		ID:          "cfg-1",
		Name:        "cfg",
		SandboxType: string(sandbox.SandboxTypeE2B),
		Config: &types.TenantSandboxConfig{
			SandboxType: string(sandbox.SandboxTypeE2B),
			E2B: &types.E2BSandboxConfig{
				APIURL:     "https://e2b.example",
				APIKey:     "key-1",
				TemplateID: "base-template",
			},
		},
	}}
	fx.skillRepo = newInstallSkillRepo()
	require.NoError(t, fx.skillRepo.CreateSkill(context.Background(), &types.TenantSkillEntity{
		ID:              "sk-1",
		SandboxConfigID: "cfg-1",
		InstallRunID:    "run-1",
		Name:            fx.bundle.Name,
		Version:         fx.bundle.Version,
		Description:     fx.bundle.Description,
		Instructions:    fx.bundle.Instructions,
		BundleSHA256:    fx.bundle.SHA256,
		Enabled:         true,
		Status:          types.SkillStatusInstalling,
	}))

	fx.sandboxMgr = &installSandboxManager{fx: fx}
	fx.modelSvc = &installModelService{fx: fx}
	platformAgents := NewPlatformAgentService(&installPlatformAgentRepository{fx: fx}, nil)
	sessions := &installSessionRepository{fx: fx}
	pinner := NewSessionSandboxPinner(newPinTestDB(t))
	_, err := pinner.Pin(context.Background(), "s-1", "cfg-1")
	require.NoError(t, err)
	fx.svc = NewTenantSkillService(
		fx.skillRepo,
		fx.configRepo,
		&installStorageResolver{fx: fx},
		&installSandboxResolver{mgr: fx.sandboxMgr},
		&installPlatformExecutionFactory{mgr: fx.sandboxMgr},
		stubWorkspaceSandboxPolicy{},
		sessions,
		pinner,
		platformAgents,
		fx.modelSvc,
		nil,
		&transcriptStreams{},
		nil,
		nil,
	)
	fx.svc.now = func() time.Time { return time.Date(2026, 8, 19, 9, 30, 0, 0, time.UTC) }
	return fx
}

func (f *installFixture) record(event string) {
	f.events = append(f.events, event)
}

// nextLoadCheckExit returns the code for this verification pass, repeating the
// last configured entry once they run out.
func (f *installFixture) nextLoadCheckExit() int {
	f.loadCheckPasses++
	if len(f.loadCheckExitCodes) == 0 {
		return 0
	}
	if f.loadCheckPasses > len(f.loadCheckExitCodes) {
		return f.loadCheckExitCodes[len(f.loadCheckExitCodes)-1]
	}
	return f.loadCheckExitCodes[f.loadCheckPasses-1]
}

// now is the fixture's clock, so a test can express "one heartbeat ago"
// against the same instant the service reads.
func (f *installFixture) now() time.Time { return f.svc.now() }

// seedInstalledSkill puts the fixture in the state a removal starts from: the
// skill is ready inside the config's current image, the ledger holds the active
// row that produced that image, and the image manifest lists the skill.
func (f *installFixture) seedInstalledSkill(skillID, snapshotID string, generation int) {
	f.t.Helper()
	ctx := context.Background()

	skill, err := f.skillRepo.GetSkill(ctx, "cfg-1", skillID)
	require.NoError(f.t, err)
	if skill == nil {
		skill = &types.TenantSkillEntity{
			ID: skillID, SandboxConfigID: "cfg-1",
			Name: "skill-" + skillID, Version: "1.0.0", Enabled: true,
		}
	}
	skill.Status = types.SkillStatusRemoving
	skill.InstalledSnapshotID = snapshotID
	skill.BundleRef = "storage://" + skillID + ".zip"
	require.NoError(f.t, f.skillRepo.CreateSkill(ctx, skill))

	require.NoError(f.t, f.skillRepo.CreateSnapshotRow(ctx, &types.TenantSkillSnapshotEntity{
		ID: "row-" + skillID, SandboxConfigID: "cfg-1", SkillID: skillID,
		SnapshotID: snapshotID, Generation: generation,
		Trigger: types.SkillSnapshotTriggerInstall, State: types.SkillSnapshotStateActive,
	}))

	f.configRepo.entity.Config.SkillImage = &types.SkillImageConfig{
		SnapshotID: snapshotID, Generation: generation,
		BaseTemplateID: "base-template", OwnerFingerprint: f.fingerprint,
	}

	f.manifest.Skills = append(f.manifest.Skills, skillImageManifestEntry{
		ID: skillID, Name: skill.Name, Version: skill.Version,
		SHA256: strings.Repeat("b", 64),
	})
	payload, err := json.Marshal(f.manifest)
	require.NoError(f.t, err)
	f.sandboxMgr.manifest = payload
}

// seedReadySkillWithSHA puts the fixture in the state a no-op re-upload starts
// from: the skill is ready, the digest matches the archive about to be posted,
// and the ledger says those files are still on the live image.
func (f *installFixture) seedReadySkillWithSHA(sha256, snapshotID string) {
	f.t.Helper()
	ctx := context.Background()
	skill, err := f.skillRepo.GetSkill(ctx, "cfg-1", "sk-1")
	require.NoError(f.t, err)
	require.NotNil(f.t, skill)
	skill.Status = types.SkillStatusReady
	skill.BundleSHA256 = sha256
	skill.InstalledSnapshotID = snapshotID
	skill.Error = ""
	skill.InstallingSince = nil
	require.NoError(f.t, f.skillRepo.UpdateSkill(ctx, skill))

	require.NoError(f.t, f.skillRepo.CreateSnapshotRow(ctx, &types.TenantSkillSnapshotEntity{
		ID: "row-live", SandboxConfigID: "cfg-1", SkillID: "sk-1",
		SnapshotID: snapshotID, Generation: 1,
		Trigger: types.SkillSnapshotTriggerInstall, State: types.SkillSnapshotStateActive,
	}))
	f.configRepo.entity.Config.SkillImage = &types.SkillImageConfig{
		SnapshotID: snapshotID, Generation: 1,
		BaseTemplateID: "base-template", OwnerFingerprint: f.fingerprint,
	}
}

type installConfigRepo struct {
	fx        *installFixture
	entity    *types.TenantSandboxConfigEntity
	saved     *types.TenantSandboxConfigEntity
	updateErr error
	updates   int
	reads     int
	// editAfterFirstRead simulates an admin editing the config through the
	// config service (which is serialised by its own cordon, not by the skill
	// image lock) while the install is running.
	editAfterFirstRead func(*types.TenantSandboxConfigEntity)
}

func (r *installConfigRepo) Create(context.Context, *types.TenantSandboxConfigEntity) error {
	return nil
}

func (r *installConfigRepo) GetByID(
	_ context.Context, id string,
) (*types.TenantSandboxConfigEntity, error) {
	if r.entity == nil || r.entity.ID != id {
		return nil, nil
	}
	cp := *r.entity
	if r.entity.Config != nil {
		cfg := *r.entity.Config
		if r.entity.Config.E2B != nil {
			e2b := *r.entity.Config.E2B
			cfg.E2B = &e2b
		}
		if r.entity.Config.Cube != nil {
			cube := *r.entity.Config.Cube
			cfg.Cube = &cube
		}
		if r.entity.Config.SkillImage != nil {
			image := *r.entity.Config.SkillImage
			cfg.SkillImage = &image
		}
		cp.Config = &cfg
	}
	r.reads++
	if r.reads == 1 && r.editAfterFirstRead != nil {
		r.editAfterFirstRead(r.entity)
	}
	if r.reads == 1 && r.fx != nil && r.fx.cancelDuringConfigRead != nil {
		r.fx.cancelDuringConfigRead()
	}
	return &cp, nil
}

// ListAll returns the one config this fixture holds, so a housekeeping scan
// sees the same config the install and removal tests act on.
func (r *installConfigRepo) ListAll(context.Context) ([]*types.TenantSandboxConfigEntity, error) {
	if r.entity == nil {
		return nil, nil
	}
	cp := *r.entity
	return []*types.TenantSandboxConfigEntity{&cp}, nil
}

func (r *installConfigRepo) GetDefault(context.Context) (*types.TenantSandboxConfigEntity, error) {
	if r.entity == nil || !r.entity.IsDefault {
		return nil, nil
	}
	cp := *r.entity
	return &cp, nil
}

func (r *installConfigRepo) SetDefault(_ context.Context, id string) error {
	if r.entity != nil {
		r.entity.IsDefault = r.entity.ID == id
	}
	return nil
}

// Update honours the context because the real gorm repository does: the
// pointer switch is the one write a lost lock must not be able to complete.
func (r *installConfigRepo) Update(ctx context.Context, e *types.TenantSandboxConfigEntity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updates++
	if r.fx != nil {
		r.fx.record("switch-pointer")
	}
	cp := *e
	r.saved = &cp
	r.entity = &cp
	return nil
}

func (r *installConfigRepo) SoftDelete(context.Context, string) error { return nil }
func (r *installConfigRepo) SetCordon(context.Context, string, time.Time) error {
	return nil
}
func (r *installConfigRepo) ClearCordon(context.Context, string) error { return nil }

type installSkillRepo struct {
	mu        sync.Mutex
	skills    map[string]*types.TenantSkillEntity
	snapshots map[string]*types.TenantSkillSnapshotEntity
	catalogs  map[string]*types.TenantSkillCatalogEntity
	// updateFailsWhen models a transient write failure for one kind of row
	// state, so a test can fail the terminal bookkeeping write without
	// disabling every other write the run makes.
	updateFailsWhen func(*types.TenantSkillEntity) bool
	// markStateFails models the ledger write that records a just-created
	// snapshot failing for one state, leaving the snapshot ID nowhere but a
	// local variable.
	markStateFails func(state string) bool
	// listSnapshotsErr models an unreadable ledger, which is what stands
	// between "the image still carries this skill" and a guess.
	listSnapshotsErr error
	// deleteSkillErr models the row delete failing past the point of no
	// return.
	deleteSkillErr error
	createErr      error
	// updateCatalogErr models the definition row failing to commit after its
	// new archive is already stored.
	updateCatalogErr error
	// createCatalogHook stands in for the insert so a test can have the unique
	// index reject this row because another request won the name first.
	createCatalogHook   func(*types.TenantSkillCatalogEntity) error
	getByNameMisses     int
	readyWriteAttempts  int
	deleteSkillAttempts int
	// listCalls counts attempts, not successes: a caller that gave up before
	// listing and one whose listing failed are different bugs, and the skill
	// derivation tests turn on telling them apart.
	listCalls int
	// userEnvs is a real store rather than a stub: the env-var flows are about
	// which principal's value wins, so a fake that cannot distinguish
	// principals would let the interesting bugs through.
	userEnvs []*types.TenantUserEnvVar
}

func newInstallSkillRepo() *installSkillRepo {
	return &installSkillRepo{
		skills:    map[string]*types.TenantSkillEntity{},
		snapshots: map[string]*types.TenantSkillSnapshotEntity{},
		catalogs:  map[string]*types.TenantSkillCatalogEntity{},
	}
}

func skillKey(_ uint64, configID, skillID string) string {
	return configID + "|" + skillID
}

func (r *installSkillRepo) CreateSkill(_ context.Context, e *types.TenantSkillEntity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	cp := *e
	r.skills[skillKey(0, e.SandboxConfigID, e.ID)] = &cp
	return nil
}

// GetSkill and UpdateSkill honour the context because the real gorm repository
// does: a write attempted on a cancelled context never reaches the database.
func (r *installSkillRepo) GetSkill(
	ctx context.Context, configID, skillID string,
) (*types.TenantSkillEntity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.skills[skillKey(0, configID, skillID)]
	if e == nil {
		return nil, nil
	}
	cp := *e
	return &cp, nil
}

func (r *installSkillRepo) GetSkillByName(
	_ context.Context, configID, name string,
) (*types.TenantSkillEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getByNameMisses > 0 {
		r.getByNameMisses--
		return nil, nil
	}
	for _, e := range r.skills {
		if e.SandboxConfigID == configID && e.Name == name {
			cp := *e
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *installSkillRepo) ListSkillsByConfig(
	ctx context.Context, configID string,
) ([]*types.TenantSkillEntity, error) {
	// Counted before the context check so a cancelled listing still registers
	// as an attempt.
	r.mu.Lock()
	r.listCalls++
	r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*types.TenantSkillEntity
	for _, e := range r.skills {
		if e.SandboxConfigID == configID {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *installSkillRepo) UpdateSkill(ctx context.Context, e *types.TenantSkillEntity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.Status == types.SkillStatusReady {
		r.readyWriteAttempts++
	}
	if r.updateFailsWhen != nil && r.updateFailsWhen(e) {
		return errUpdateBoom
	}
	key := skillKey(0, e.SandboxConfigID, e.ID)
	cp := *e
	// The real UpdateSkill leaves the envs column alone, so a stale in-memory
	// copy cannot put an old declaration back. The fake has to model that or
	// the tests would pass on behaviour production does not have.
	if stored := r.skills[key]; stored != nil {
		cp.Envs = stored.Envs
		cp.Enabled = stored.Enabled
		cp.InstallRunID = stored.InstallRunID
		cp.InstallTranscript = append(types.JSON(nil), stored.InstallTranscript...)
	} else {
		cp.Envs = nil
	}
	r.skills[key] = &cp
	return nil
}

func (r *installSkillRepo) BeginSkillRun(
	ctx context.Context, configID, skillID, runID, status string, startedAt time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.skills[skillKey(0, configID, skillID)]
	if stored == nil {
		return nil
	}
	stored.InstallRunID = runID
	if status == types.SkillStatusInstalling {
		stored.InstallTranscript = nil
	}
	stored.Status = status
	stored.Error = ""
	stored.InstallingSince = &startedAt
	return nil
}

func (r *installSkillRepo) UpdateSkillForRun(
	ctx context.Context, e *types.TenantSkillEntity, runID string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := skillKey(0, e.SandboxConfigID, e.ID)
	stored := r.skills[key]
	if stored == nil || stored.InstallRunID != runID {
		return false, nil
	}
	if e.Status == types.SkillStatusReady {
		r.readyWriteAttempts++
	}
	if r.updateFailsWhen != nil && r.updateFailsWhen(e) {
		return false, errUpdateBoom
	}
	cp := *e
	cp.Envs = stored.Envs
	cp.Enabled = stored.Enabled
	cp.InstallRunID = stored.InstallRunID
	cp.InstallTranscript = append(types.JSON(nil), stored.InstallTranscript...)
	r.skills[key] = &cp
	return true, nil
}

func (r *installSkillRepo) UpdateInstallTranscript(
	ctx context.Context, configID, skillID, runID string, transcript types.JSON,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.skills[skillKey(0, configID, skillID)]
	if stored == nil || stored.InstallRunID != runID {
		return false, nil
	}
	stored.InstallTranscript = append(types.JSON(nil), transcript...)
	return true, nil
}

func (r *installSkillRepo) UpdateSkillEnvs(
	ctx context.Context, configID, skillID string, envs types.SkillEnvVars,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.skills[skillKey(0, configID, skillID)]
	if stored == nil {
		return nil
	}
	stored.Envs = envs
	return nil
}

func (r *installSkillRepo) UpdateSkillEnvsForRun(
	ctx context.Context, configID, skillID, runID string, envs types.SkillEnvVars,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.skills[skillKey(0, configID, skillID)]
	if stored == nil || stored.InstallRunID != runID {
		return false, nil
	}
	stored.Envs = envs
	return true, nil
}

func (r *installSkillRepo) UpdateSkillAdminState(
	ctx context.Context,
	configID, skillID string,
	enabled bool,
	envs types.SkillEnvVars,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.skills[skillKey(0, configID, skillID)]
	if stored == nil {
		return nil
	}
	stored.Enabled = enabled
	stored.Envs = envs
	return nil
}

// DeleteSkill honours the context and really drops the row, because the
// removal flow's whole contract is that the row disappears only once the image
// no longer carries the skill.
func (r *installSkillRepo) DeleteSkill(
	ctx context.Context, configID, skillID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteSkillAttempts++
	if r.deleteSkillErr != nil {
		return r.deleteSkillErr
	}
	key := skillKey(0, configID, skillID)
	if _, matched := r.skills[key]; !matched {
		// Mirrors the real repository: when the scoped key matches no skill,
		// nothing is deleted at all. Deleting the user values here anyway would
		// hide the exact data-loss bug the real DeleteSkill was fixed to avoid.
		return nil
	}
	delete(r.skills, key)
	return nil
}

func (r *installSkillRepo) DeleteSkillForRun(
	ctx context.Context, configID, skillID, runID string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := skillKey(0, configID, skillID)
	stored := r.skills[key]
	if stored == nil || stored.InstallRunID != runID {
		return false, nil
	}
	r.deleteSkillAttempts++
	if r.deleteSkillErr != nil {
		return false, r.deleteSkillErr
	}
	delete(r.skills, key)
	return true, nil
}

func (r *installSkillRepo) ListStaleInstalling(context.Context, time.Time) ([]*types.TenantSkillEntity, error) {
	return nil, nil
}

func (r *installSkillRepo) CreateSnapshotRow(_ context.Context, e *types.TenantSkillSnapshotEntity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *e
	r.snapshots[e.ID] = &cp
	return nil
}

func (r *installSkillRepo) MarkSnapshotState(
	_ context.Context, id, state, snapshotID string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.markStateFails != nil && r.markStateFails(state) {
		return errUpdateBoom
	}
	e := r.snapshots[id]
	if e == nil {
		return nil
	}
	e.State = state
	if snapshotID != "" {
		e.SnapshotID = snapshotID
	}
	return nil
}

func (r *installSkillRepo) ListSnapshotsByConfig(
	_ context.Context, configID string,
) ([]*types.TenantSkillSnapshotEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listSnapshotsErr != nil {
		return nil, r.listSnapshotsErr
	}
	var out []*types.TenantSkillSnapshotEntity
	for _, e := range r.snapshots {
		if e.SandboxConfigID == configID {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *installSkillRepo) DeleteSnapshotRowsByConfig(context.Context, string) error {
	return nil
}

func (r *installSkillRepo) ListSkills(ctx context.Context) ([]*types.TenantSkillEntity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*types.TenantSkillEntity
	for _, e := range r.skills {
		cp := *e
		out = append(out, &cp)
	}
	return out, nil
}

func (r *installSkillRepo) CountUserEnvVarsByConfig(
	_ context.Context, configID string,
) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for _, row := range r.userEnvs {
		if row.SandboxConfigID == configID {
			count++
		}
	}
	return count, nil
}

// userEnvMatches mirrors the real repository's unique index, minus the name,
// which the per-row callers add.
func userEnvMatches(
	e *types.TenantUserEnvVar, tenantID uint64, p types.Principal, configID, skillID string,
) bool {
	p = p.Normalize()
	return e.TenantID == tenantID && e.PrincipalType == p.Type &&
		e.PrincipalID == p.ID && e.SandboxConfigID == configID && e.SkillID == skillID
}

func (r *installSkillRepo) ListUserEnvVars(
	ctx context.Context, tenantID uint64, p types.Principal, configID, skillID string,
) ([]*types.TenantUserEnvVar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*types.TenantUserEnvVar
	for _, e := range r.userEnvs {
		if userEnvMatches(e, tenantID, p, configID, skillID) {
			cp := *e
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *installSkillRepo) ListUserEnvVarsByConfig(
	ctx context.Context, tenantID uint64, p types.Principal, configID string,
) ([]*types.TenantUserEnvVar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p = p.Normalize()
	var out []*types.TenantUserEnvVar
	for _, e := range r.userEnvs {
		if e.TenantID == tenantID && e.PrincipalType == p.Type &&
			e.PrincipalID == p.ID && e.SandboxConfigID == configID {
			cp := *e
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SkillID != out[j].SkillID {
			return out[i].SkillID < out[j].SkillID
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (r *installSkillRepo) UpsertUserEnvVar(ctx context.Context, e *types.TenantUserEnvVar) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p := types.Principal{Type: e.PrincipalType, ID: e.PrincipalID}.Normalize()
	for _, existing := range r.userEnvs {
		if userEnvMatches(existing, e.TenantID, p, e.SandboxConfigID, e.SkillID) &&
			existing.Name == e.Name {
			existing.Value = e.Value
			return nil
		}
	}
	cp := *e
	cp.PrincipalType, cp.PrincipalID = p.Type, p.ID
	if cp.ID == "" {
		cp.ID = uuid.NewString()
	}
	r.userEnvs = append(r.userEnvs, &cp)
	return nil
}

func (r *installSkillRepo) DeleteUserEnvVar(
	ctx context.Context, tenantID uint64, p types.Principal, configID, skillID, name string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.userEnvs {
		if userEnvMatches(e, tenantID, p, configID, skillID) && e.Name == name {
			r.userEnvs = append(r.userEnvs[:i], r.userEnvs[i+1:]...)
			return nil
		}
	}
	return types.ErrEnvVarNotFound
}

func (r *installSkillRepo) DeleteUserEnvVarsByConfig(
	ctx context.Context, tenantID uint64, configID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.userEnvs[:0]
	for _, e := range r.userEnvs {
		if e.TenantID == tenantID && e.SandboxConfigID == configID {
			continue
		}
		kept = append(kept, e)
	}
	r.userEnvs = kept
	return nil
}

func (r *installSkillRepo) CreateCatalog(_ context.Context, e *types.TenantSkillCatalogEntity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.catalogs == nil {
		r.catalogs = map[string]*types.TenantSkillCatalogEntity{}
	}
	if r.createCatalogHook != nil {
		return r.createCatalogHook(e)
	}
	cp := *e
	r.catalogs[e.ID] = &cp
	return nil
}

func (r *installSkillRepo) GetCatalog(_ context.Context, catalogID string) (*types.TenantSkillCatalogEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.catalogs[catalogID]
	if stored == nil {
		return nil, nil
	}
	cp := *stored
	return &cp, nil
}

func (r *installSkillRepo) GetCatalogByName(_ context.Context, name string) (*types.TenantSkillCatalogEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.catalogs {
		if row.Name == name {
			cp := *row
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *installSkillRepo) ListCatalogs(_ context.Context) ([]*types.TenantSkillCatalogEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*types.TenantSkillCatalogEntity
	for _, row := range r.catalogs {
		cp := *row
		out = append(out, &cp)
	}
	return out, nil
}

func (r *installSkillRepo) UpdateCatalog(_ context.Context, e *types.TenantSkillCatalogEntity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updateCatalogErr != nil {
		return r.updateCatalogErr
	}
	if r.catalogs == nil {
		r.catalogs = map[string]*types.TenantSkillCatalogEntity{}
	}
	cp := *e
	r.catalogs[e.ID] = &cp
	return nil
}

func (r *installSkillRepo) DeleteCatalog(_ context.Context, catalogID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.catalogs, catalogID)
	return nil
}

func (r *installSkillRepo) ListSkillsByCatalog(_ context.Context, catalogID string) ([]*types.TenantSkillEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*types.TenantSkillEntity
	for _, row := range r.skills {
		if row.CatalogID == catalogID {
			cp := *row
			out = append(out, &cp)
		}
	}
	return out, nil
}

var _ repository.TenantSkillRepository = (*installSkillRepo)(nil)

type installSandboxResolver struct {
	mgr sandbox.Manager
}

func (r *installSandboxResolver) Resolve(context.Context, uint64, string) (sandbox.Manager, error) {
	return r.mgr, nil
}

type installPlatformExecutionFactory struct {
	mgr *installSandboxManager
}

func (f *installPlatformExecutionFactory) StartPlatformSkillRun(
	_ context.Context, _, _ string,
) (sandbox.PlatformSkillExecution, error) {
	f.mgr.fx.record("create-session")
	return f.mgr, nil
}

type installSandboxManager struct {
	fx            *installFixture
	structureSeen bool
	writes        []string
	// writeContents is what actually landed in the image, keyed by path. The
	// manifest rewrite is only meaningful as content, not as a path.
	writeContents map[string][]byte
	// manifest is the file the image already carries at SkillsManifestPath.
	manifest []byte
	// files are the other files the image already carries, keyed by absolute
	// path. The env declaration crosses from the sandbox to the server as one
	// of these, never as a tool call.
	files map[string][]byte
}

func (m *installSandboxManager) sortedWrites() []string {
	out := append([]string(nil), m.writes...)
	sort.Strings(out)
	return out
}

func (m *installSandboxManager) Execute(context.Context, *sandbox.ExecuteConfig) (*sandbox.ExecuteResult, error) {
	return &sandbox.ExecuteResult{ExitCode: 0}, nil
}
func (m *installSandboxManager) Cleanup(context.Context) error {
	m.fx.destroyedSandboxes = append(m.fx.destroyedSandboxes, m.SandboxID())
	m.fx.record("destroy-sandbox")
	return nil
}
func (m *installSandboxManager) GetSandbox() sandbox.Sandbox                        { return nil }
func (m *installSandboxManager) GetType() sandbox.SandboxType                       { return sandbox.SandboxTypeE2B }
func (m *installSandboxManager) SandboxID() string                                  { return "sess-1" }
func (m *installSandboxManager) Provider() sandbox.RemoteProvider                   { return sandbox.SandboxTypeE2B }
func (m *installSandboxManager) SessionShellExecutor() sandbox.SessionShellExecutor { return m }
func (m *installSandboxManager) SessionFileStore() sandbox.SessionFileStore         { return m }
func (m *installSandboxManager) SessionInstallShellExecutor() sandbox.SessionInstallShellExecutor {
	return m
}

func (m *installSandboxManager) EnsureSessionDir(context.Context, string, string) error {
	return nil
}

func (m *installSandboxManager) ListSessionFiles(context.Context, string, string) ([]sandbox.RemoteDirEntry, error) {
	return nil, nil
}

func (m *installSandboxManager) StatSessionFile(context.Context, string, string) (*sandbox.RemoteStatEntry, error) {
	return nil, nil
}

func (m *installSandboxManager) ReadSessionFile(
	_ context.Context, _ string, filePath string,
) ([]byte, error) {
	if filePath == sandbox.SkillsManifestPath && m.manifest != nil {
		return m.manifest, nil
	}
	if content, ok := m.files[filePath]; ok {
		return content, nil
	}
	// A miss is reported the way a real backend reports one. Returning empty
	// bytes with no error would make "the agent wrote nothing" indistinguishable
	// from "the agent wrote an empty file".
	return nil, os.ErrNotExist
}

func (m *installSandboxManager) WriteSessionInputFile(
	_ context.Context, _ string, filePath string, content []byte,
) error {
	return m.WriteSessionFile(context.Background(), "", filePath, content)
}

func (m *installSandboxManager) WriteSessionWorkspaceFile(
	ctx context.Context, sessionID, filePath string, content []byte,
) error {
	return m.WriteSessionFile(ctx, sessionID, filePath, content)
}

func (m *installSandboxManager) WriteSessionFile(
	_ context.Context, _ string, filePath string, content []byte,
) error {
	m.writes = append(m.writes, filePath)
	if m.writeContents == nil {
		m.writeContents = map[string][]byte{}
	}
	m.writeContents[filePath] = content
	if filePath == sandbox.SkillsManifestPath {
		m.fx.record("write-manifest")
		return nil
	}
	if !containsEvent(m.fx.events, "seed-files") {
		if m.fx.beforeSeed != nil {
			m.fx.beforeSeed()
		}
		m.fx.record("seed-files")
	}
	return nil
}

func (m *installSandboxManager) extractSeedArchive(command string) {
	archive, ok := m.writeContents[skillSeedArchivePath]
	if !ok {
		return
	}
	skillDir := installSkillDir
	if _, after, found := strings.Cut(command, " -C "); found {
		dir, _, _ := strings.Cut(strings.TrimSpace(after), " ")
		if dir != "" {
			skillDir = dir
		}
	}
	tr := tar.NewReader(bytes.NewReader(archive))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != 0 {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return
		}
		dest := path.Join(skillDir, hdr.Name)
		m.writes = append(m.writes, dest)
		if m.writeContents == nil {
			m.writeContents = map[string][]byte{}
		}
		m.writeContents[dest] = content
	}
	delete(m.writeContents, skillSeedArchivePath)
	kept := m.writes[:0]
	for _, w := range m.writes {
		if w != skillSeedArchivePath {
			kept = append(kept, w)
		}
	}
	m.writes = kept
}

func (m *installSandboxManager) RemoveSessionInputPath(context.Context, string, string) error {
	return nil
}

func (m *installSandboxManager) ExecShellCommand(
	ctx context.Context,
	sessionID string,
	command string,
	workDir string,
	timeout time.Duration,
	env map[string]string,
) (*sandbox.ExecuteResult, error) {
	return m.ExecShellCommandWithOptions(ctx, sessionID, command, sandbox.ShellExecOptions{
		WorkDir: workDir,
		Timeout: timeout,
		Env:     env,
	})
}

// ExecShellCommandWithOptions matches on the exact command text. Substring
// matching used to let one arm shadow every later command that happened to
// contain the same words, which is how a malformed smoke command stayed
// invisible to the suite.
func (m *installSandboxManager) ExecShellCommandWithOptions(
	_ context.Context, _ string, command string, opts sandbox.ShellExecOptions,
) (*sandbox.ExecuteResult, error) {
	m.fx.commands = append(m.fx.commands, command)
	switch {
	case command == installPrepareCommand:
		m.fx.record("prepare-skill-dir")
	case strings.HasPrefix(command, "tar -xf "):
		m.extractSeedArchive(command)
	case command == skillTreeVerifyCommand(installSkillDir, []string{"scripts/extract.py"}):
		if !m.structureSeen {
			m.fx.record("verify-structure")
			m.structureSeen = true
		}
	case strings.HasPrefix(command, "test -x ") || strings.HasPrefix(command, "test -d "):
		m.fx.record("verify-deps")
		if m.fx.depsExitCode != 0 {
			return &sandbox.ExecuteResult{ExitCode: m.fx.depsExitCode, Stderr: "deps missing"}, nil
		}
	case command == installPythonVerifyCommand:
		m.fx.loadCheckRanAsRoot = opts.AsRoot
		m.fx.record("verify-python")
		if m.fx.loadCheckResult != nil {
			return m.fx.loadCheckResult, nil
		}
		return &sandbox.ExecuteResult{
			ExitCode: m.fx.nextLoadCheckExit(),
			Stdout:   m.fx.loadCheckStdout,
			Stderr:   m.fx.loadCheckStderr,
		}, nil
	case command == removeSkillDirCommand:
		m.fx.record("remove-skill-dir")
		if m.fx.removeDelay > 0 {
			time.Sleep(m.fx.removeDelay)
		}
		if m.fx.cancelDuringRemove != nil {
			m.fx.cancelDuringRemove()
		}
		return &sandbox.ExecuteResult{
			ExitCode: m.fx.rmExitCode, Stderr: "rm: cannot remove",
		}, nil
	case command == cleanImageScratchCommand():
		m.fx.record("cleanup-workspace")
	case strings.HasPrefix(command, "chmod -R 555 "):
		m.fx.record("chmod")
	}
	if m.fx.execResult != nil && command == m.fx.execResultCommand {
		return m.fx.execResult, nil
	}
	return &sandbox.ExecuteResult{ExitCode: 0}, nil
}

func (m *installSandboxManager) CreateSnapshot(context.Context, string, string) (sandbox.RemoteSnapshotRef, error) {
	m.fx.record("create-snapshot")
	if m.fx.cancelDuringSnapshot != nil {
		m.fx.cancelDuringSnapshot()
	}
	return sandbox.RemoteSnapshotRef{ID: "snap-1"}, nil
}

// DeleteSnapshot refuses a cancelled context for the same reason
// DestroySession does: it is a provider call, and it is the compensation for a
// pointer switch that a cancelled context is one of the reasons for failing.
func (m *installSandboxManager) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.fx.deletedSnapshots = append(m.fx.deletedSnapshots, snapshotID)
	return nil
}

func (m *installSandboxManager) ListSnapshots(context.Context, string) ([]sandbox.RemoteSnapshotRef, error) {
	return nil, nil
}

// InvalidateConfigSandboxes refuses a cancelled context exactly as the
// Redis-backed binding store would, so a caller that forgot to detach the
// install's context fails here rather than silently marking nothing.
func (m *installSandboxManager) InvalidateConfigSandboxes(
	ctx context.Context, tenantID uint64, configID string,
) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if m.fx.invalidateErr != nil {
		return 0, m.fx.invalidateErr
	}
	m.fx.staleMarks = append(m.fx.staleMarks, staleMark{tenantID: tenantID, configID: configID})
	m.fx.record("mark-stale")
	return 1, nil
}

// DestroySession refuses a cancelled context because the provider call does:
// releasing the sandbox is the compensation for a lost lock, so it must not run
// on the context the lock cancelled.
func (m *installSandboxManager) DestroySession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.fx.destroyedSandboxes = append(m.fx.destroyedSandboxes, sessionID)
	m.fx.record("destroy-sandbox")
	return nil
}

func containsEvent(events []string, needle string) bool {
	for _, event := range events {
		if event == needle {
			return true
		}
	}
	return false
}

type installPlatformAgentRepository struct {
	interfaces.CustomAgentRepository
	fx *installFixture
}

func (r *installPlatformAgentRepository) GetAgentByID(
	_ context.Context, id string, tenantID uint64,
) (*types.CustomAgent, error) {
	if tenantID != platformAgentTenantID || id != types.BuiltinSkillInstallerID ||
		r.fx == nil || r.fx.installerRecord == nil {
		return nil, repository.ErrCustomAgentNotFound
	}
	cp := *r.fx.installerRecord
	return &cp, nil
}

type installSessionRepository struct {
	interfaces.SessionRepository
	fx *installFixture
}

func (s *installSessionRepository) Create(
	_ context.Context, session *types.Session,
) (*types.Session, error) {
	s.fx.record("create-session")
	s.fx.sessionCalls = append(s.fx.sessionCalls, "Create")
	if session.ID == "" {
		session.ID = "sess-1"
	}
	return session, nil
}

type installModelService struct {
	fx *installFixture
	// missing names models the workspace can no longer resolve, e.g. one the
	// installer agent still points at after it was deleted.
	missing map[string]bool
}

func (s *installModelService) CreateModel(context.Context, *types.Model) error { return nil }
func (s *installModelService) GetModelByID(context.Context, string) (*types.Model, error) {
	return nil, nil
}

func (s *installModelService) ListModels(context.Context) ([]*types.Model, error) {
	return []*types.Model{{
		ID:        "model-1",
		Type:      types.ModelTypeKnowledgeQA,
		Status:    types.ModelStatusActive,
		IsDefault: true,
	}}, nil
}
func (s *installModelService) UpdateModel(context.Context, *types.Model) error { return nil }
func (s *installModelService) DeleteModel(context.Context, string) error       { return nil }
func (s *installModelService) UpdateModelCredentials(context.Context, string, *string, *string) (*types.Model, error) {
	return nil, nil
}

func (s *installModelService) ClearModelCredential(context.Context, string, string) error {
	return nil
}

func (s *installModelService) GetEmbeddingModel(context.Context, string) (embedding.Embedder, error) {
	return nil, nil
}

func (s *installModelService) GetEmbeddingModelForTenant(context.Context, string, uint64) (embedding.Embedder, error) {
	return nil, nil
}

func (s *installModelService) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return nil, nil
}

func (s *installModelService) GetChatModel(_ context.Context, modelID string) (chat.Chat, error) {
	if s.missing[modelID] {
		return nil, fmt.Errorf("model %s not found", modelID)
	}
	return installChat{id: modelID, fx: s.fx}, nil
}
func (s *installModelService) GetVLMModel(context.Context, string) (vlm.VLM, error) { return nil, nil }
func (s *installModelService) GetASRModel(context.Context, string) (asr.ASR, error) { return nil, nil }

type installChat struct {
	id string
	fx *installFixture
}

func (installChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	return nil, nil
}

func (c installChat) ChatStream(_ context.Context, messages []chat.Message, _ *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	if c.fx != nil {
		if c.fx.beforeExecute != nil {
			c.fx.beforeExecute()
		}
		if len(messages) > 0 {
			c.fx.agentPrompts = append(c.fx.agentPrompts, messages[len(messages)-1].Content)
		}
		c.fx.record("agent-execute")
		if c.fx.agentDelay > 0 {
			time.Sleep(c.fx.agentDelay)
		}
		if c.fx.agentErr != nil {
			return nil, c.fx.agentErr
		}
	}
	stream := make(chan types.StreamResponse, 1)
	stream <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content:      "installation complete",
		Done:         true,
		FinishReason: "stop",
	}
	close(stream)
	return stream, nil
}
func (installChat) GetModelName() string { return "install-chat" }
func (c installChat) GetModelID() string { return c.id }

type installStorageResolver struct{ fx *installFixture }

func (r *installStorageResolver) Put(_ context.Context, _ string, data []byte) (string, error) {
	return installFileService{fx: r.fx}.SaveBytes(context.Background(), data, 0, "", false)
}

func (r *installStorageResolver) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	return installFileService{fx: r.fx}.GetFile(ctx, ref)
}

func (r *installStorageResolver) Delete(_ context.Context, ref string) error {
	if r.fx != nil {
		r.fx.deletedBundles = append(r.fx.deletedBundles, ref)
		delete(r.fx.storedBundles, ref)
	}
	return nil
}

func (r *installStorageResolver) ResolveFileService(
	context.Context, string, string,
) (interfaces.FileService, string, error) {
	return installFileService{fx: r.fx}, "", nil
}

func (r *installStorageResolver) ResolveBackend(
	context.Context, string,
) (*types.StorageBackend, error) {
	return nil, nil
}

type installFileService struct{ fx *installFixture }

func (installFileService) CheckConnectivity(context.Context) error { return nil }
func (installFileService) SaveFile(context.Context, *multipart.FileHeader, uint64, string) (string, error) {
	return "", nil
}

func (s installFileService) SaveBytes(_ context.Context, data []byte, _ uint64, _ string, _ bool) (string, error) {
	if s.fx != nil {
		if s.fx.beforeSaveBundle != nil {
			s.fx.beforeSaveBundle()
		}
		s.fx.savedBundles++
		if s.fx.saveErr != nil {
			return "", s.fx.saveErr
		}
		if s.fx.storedBundles == nil {
			s.fx.storedBundles = map[string][]byte{}
		}
		copied := make([]byte, len(data))
		copy(copied, data)
		ref := fmt.Sprintf("storage://bundle-%d.zip", s.fx.savedBundles)
		s.fx.storedBundles[ref] = copied
		return ref, nil
	}
	return "storage://bundle.zip", nil
}
func (s installFileService) GetFile(_ context.Context, ref string) (io.ReadCloser, error) {
	if s.fx != nil {
		s.fx.getFileCalls.Add(1)
		if data, ok := s.fx.storedBundles[ref]; ok {
			return io.NopCloser(bytes.NewReader(data)), nil
		}
	}
	return nil, errors.New("bundle not found")
}
func (installFileService) GetFileURL(context.Context, string) (string, error) { return "", nil }
func (s installFileService) DeleteFile(_ context.Context, ref string) error {
	if s.fx != nil {
		s.fx.deletedBundles = append(s.fx.deletedBundles, ref)
	}
	return nil
}

func (installFileService) CopyFile(context.Context, string, uint64, string) (string, error) {
	return "", nil
}

func TestRunInstallSessionPreparationVerifiesWithoutSnapshot(t *testing.T) {
	fx := newInstallFixture(t)
	fx.configRepo.entity.Config.SkillPreparation = "session"
	require.NoError(t, fx.svc.runInstall(context.Background(), "cfg-1", "sk-1", "run-1", fx.bundle))
	require.Contains(t, fx.events, "verify-python")
	require.NotContains(t, fx.events, "create-snapshot")
	require.Contains(t, fx.events, "destroy-sandbox")
	row, err := fx.skillRepo.GetSkill(context.Background(), "cfg-1", "sk-1")
	require.NoError(t, err)
	require.Equal(t, types.SkillStatusReady, row.Status)
	require.Empty(t, row.InstalledSnapshotID)
	require.Equal(t, 0, fx.configRepo.updates)
}
