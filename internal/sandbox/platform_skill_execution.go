package sandbox

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
)

const remoteMetadataPlatformSkillRunID = "weknora_platform_skill_run_id"

// PlatformSkillExecution is the explicit privileged port used by platform
// skill maintenance. It owns one transient provider sandbox and has no
// business session or tenant identity.
type PlatformSkillExecution interface {
	SessionInstallShellExecutor
	Cleanup(ctx context.Context) error
	CreateSnapshot(ctx context.Context, executionID, name string) (RemoteSnapshotRef, error)
	DeleteSnapshot(ctx context.Context, snapshotID string) error
	ListSnapshots(ctx context.Context, executionID string) ([]RemoteSnapshotRef, error)
	StatSessionFile(ctx context.Context, executionID, filePath string) (*RemoteStatEntry, error)
	ReadSessionFile(ctx context.Context, executionID, filePath string) ([]byte, error)
	WriteSessionFile(ctx context.Context, executionID, filePath string, content []byte) error
}

type PlatformSkillExecutionFactory interface {
	StartPlatformSkillRun(ctx context.Context, configID, runID string) (PlatformSkillExecution, error)
}

// NewPlatformSkillExecutionFactory reuses the resolver's provider clients and
// guarded transport pools without widening TenantSandboxResolver.
func NewPlatformSkillExecutionFactory(resolver TenantSandboxResolver) PlatformSkillExecutionFactory {
	factory, _ := resolver.(PlatformSkillExecutionFactory)
	return factory
}

func (r *tenantSandboxResolver) StartPlatformSkillRun(
	ctx context.Context, configID, runID string,
) (PlatformSkillExecution, error) {
	configID, runID = strings.TrimSpace(configID), strings.TrimSpace(runID)
	if configID == "" || runID == "" {
		return nil, errors.New("sandbox: platform skill run requires config and run IDs")
	}
	resolved, err := r.deps.Loader.Load(ctx, configID)
	if err != nil {
		return nil, fmt.Errorf("sandbox: load platform config %q: %w", configID, err)
	}
	if !resolved.Found {
		return nil, fmt.Errorf("%w: %s", ErrSandboxConfigNotFound, configID)
	}
	if resolved.Cordoned {
		return nil, fmt.Errorf("%w: %s", ErrSandboxConfigCordoned, configID)
	}
	effective, err := ResolveEffectiveConfig(resolved.Config, r.deps.GlobalConfig)
	if err != nil {
		return nil, err
	}
	if err := EnsureDockerBackendAllowed(effective.Type); err != nil {
		return nil, err
	}
	client, err := r.buildClient(effective)
	if err != nil {
		return nil, err
	}
	req, err := buildSessionCreateRequest(client.Provider(), effective)
	if err != nil {
		return nil, err
	}
	// Maintenance is not resumable business state. Provider expiry must destroy
	// it rather than leave a paused, billable instance.
	req.Timeout.Action = RemoteOnTimeoutKill
	req.Timeout.AutoResume = false
	req.Metadata = map[string]string{
		remoteMetadataPlatformSkillRunID: runID,
		remoteMetadataConfigID:           configID,
		remoteMetadataProvider:           string(client.Provider()),
	}
	handle, err := client.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	if handle == nil || strings.TrimSpace(handle.ID()) == "" {
		return nil, errors.New("sandbox: provider returned an empty platform skill sandbox")
	}
	return &platformSkillExecution{
		client: client, handle: handle, config: effective, runID: runID,
	}, nil
}

type platformSkillExecution struct {
	client RemoteSandboxClient
	handle RemoteSandboxHandle
	config *Config
	runID  string
	once   sync.Once
	err    error
}

func (p *platformSkillExecution) requireRun(executionID string) error {
	if p == nil || p.handle == nil || strings.TrimSpace(executionID) != p.runID {
		return errors.New("sandbox: platform skill execution binding mismatch")
	}
	return nil
}

func (p *platformSkillExecution) Cleanup(ctx context.Context) error {
	if p == nil || p.client == nil || p.handle == nil {
		return nil
	}
	p.once.Do(func() {
		p.err = p.client.Delete(ctx, p.handle.ID())
		if IsRemoteNotFound(p.err) {
			p.err = nil
		}
	})
	return p.err
}

func (p *platformSkillExecution) ExecShellCommandWithOptions(
	ctx context.Context, executionID, command string, opts ShellExecOptions,
) (*ExecuteResult, error) {
	if err := p.requireRun(executionID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("sandbox: command required for platform skill execution")
	}
	workDir := strings.TrimSpace(opts.WorkDir)
	if workDir != "" {
		clean, err := cleanSessionWorkDir(workDir, opts.AllowSkillsRoot)
		if err != nil {
			return nil, err
		}
		workDir = clean
		if err := p.client.MakeDir(ctx, p.handle, workDir); err != nil {
			return nil, err
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 && p.config != nil {
		timeout = p.config.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	user := DefaultSandboxExecUser
	if opts.AsRoot {
		user = "root"
	}
	started := time.Now()
	result, execErr := p.client.Exec(ctx, p.handle, RemoteExecRequest{
		Command: command, Shell: true, Env: opts.Env, WorkDir: workDir,
		User: user, Timeout: timeout,
	})
	return remoteExecuteResult(result, execErr, time.Since(started)), nil
}

func (p *platformSkillExecution) StatSessionFile(
	ctx context.Context, executionID, filePath string,
) (*RemoteStatEntry, error) {
	if err := p.requireRun(executionID); err != nil {
		return nil, err
	}
	return p.client.Stat(ctx, p.handle, filePath)
}

func (p *platformSkillExecution) ReadSessionFile(
	ctx context.Context, executionID, filePath string,
) ([]byte, error) {
	if err := p.requireRun(executionID); err != nil {
		return nil, err
	}
	return p.client.ReadFile(ctx, p.handle, filePath)
}

func (p *platformSkillExecution) WriteSessionFile(
	ctx context.Context, executionID, filePath string, content []byte,
) error {
	if err := p.requireRun(executionID); err != nil {
		return err
	}
	clean := path.Clean(strings.TrimSpace(filePath))
	if clean != SkillsImageRoot && !strings.HasPrefix(clean, SkillsImageRoot+"/") {
		return fmt.Errorf("sandbox: install file path %q is outside %s", filePath, SkillsImageRoot)
	}
	if err := p.client.MakeDir(ctx, p.handle, path.Dir(clean)); err != nil {
		return err
	}
	return p.client.WriteFile(ctx, p.handle, clean, content)
}

func (p *platformSkillExecution) CreateSnapshot(
	ctx context.Context, executionID, name string,
) (RemoteSnapshotRef, error) {
	if err := p.requireRun(executionID); err != nil {
		return RemoteSnapshotRef{}, err
	}
	mgr, ok := SnapshotManagerFrom(p.client)
	if !ok {
		return RemoteSnapshotRef{}, errors.New("sandbox backend does not support snapshots")
	}
	return mgr.CreateSnapshot(ctx, p.handle.ID(), name)
}

func (p *platformSkillExecution) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	mgr, ok := SnapshotManagerFrom(p.client)
	if !ok {
		return errors.New("sandbox backend does not support snapshots")
	}
	return mgr.DeleteSnapshot(ctx, snapshotID)
}

func (p *platformSkillExecution) ListSnapshots(
	ctx context.Context, executionID string,
) ([]RemoteSnapshotRef, error) {
	if executionID != "" {
		if err := p.requireRun(executionID); err != nil {
			return nil, err
		}
	}
	mgr, ok := SnapshotManagerFrom(p.client)
	if !ok {
		return nil, errors.New("sandbox backend does not support snapshots")
	}
	sandboxID := ""
	if executionID != "" {
		sandboxID = p.handle.ID()
	}
	return mgr.ListSnapshots(ctx, sandboxID)
}

var _ PlatformSkillExecutionFactory = (*tenantSandboxResolver)(nil)
var _ PlatformSkillExecution = (*platformSkillExecution)(nil)
