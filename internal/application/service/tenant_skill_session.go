package service

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// ConfigureSessionSkillPreparation wires the callback after both services exist;
// the installer itself uses AgentService, so constructor injection would cycle.
func ConfigureSessionSkillPreparation(agents interfaces.AgentService, service *TenantSkillService) {
	if concrete, ok := agents.(*agentService); ok {
		concrete.sessionSkills = service
	}
}

func (s *TenantSkillService) prepareSessionSkill(ctx context.Context, mgr sandbox.Manager,
	tenantID uint64, sessionID, configID string, selected *types.TenantSkillEntity,
	prepareExecution bool,
) error {
	if tenantID == 0 || strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("skill preparation requires a real tenant and session")
	}
	if s.sandboxPolicy == nil {
		return fmt.Errorf("sandbox permission service is unavailable")
	}
	disabled, err := s.sandboxPolicy.WorkspaceScriptsDisabled(ctx, tenantID)
	if err != nil {
		return err
	}
	if disabled {
		return fmt.Errorf("sandbox script execution is disabled for this workspace")
	}
	cfg, err := s.configs.GetByID(ctx, configID)
	if err != nil {
		return err
	}
	if cfg == nil || cfg.Config == nil {
		return fmt.Errorf("sandbox config is unavailable")
	}
	if cfg.Config.SkillPreparation != "session" {
		return nil
	}
	if s.sessions == nil {
		return fmt.Errorf("session repository is unavailable")
	}
	sess, err := s.sessions.GetByID(ctx, tenantID, sessionID)
	if err != nil {
		return err
	}
	if sess == nil || sess.TenantID != tenantID {
		return fmt.Errorf("session is outside the workspace")
	}
	execution, ok := mgr.(skillMaintenanceExecution)
	if !ok || execution == nil {
		return fmt.Errorf("sandbox backend cannot prepare skills")
	}
	// All preparation in one session shares its seed archive and package tree.
	// Reuse the existing renewable lock implementation with a session namespace.
	return s.withConfigLock(ctx, fmt.Sprintf("session:%d:%s", tenantID, sessionID), func(ctx context.Context) error {
		row, err := s.skills.GetSkill(ctx, configID, selected.ID)
		if err != nil {
			return err
		}
		if row == nil || row.SandboxConfigID != configID || !row.Enabled || row.Status != types.SkillStatusReady ||
			row.BundleSHA256 != selected.BundleSHA256 || row.Name != selected.Name {
			return fmt.Errorf("skill changed or was disabled; retry with the current skill selection")
		}
		if len(row.BundleSHA256) != 64 {
			return fmt.Errorf("skill has no verified bundle digest")
		}
		// read_skill uses persisted instructions/archive files, not the sandbox.
		// Keep the same current-state checks but leave installation to execution.
		if !prepareExecution {
			return nil
		}
		dir, err := sandbox.SkillDirFor(row.Name)
		if err != nil {
			return err
		}
		marker := path.Join(dir, ".weknora-session-ready")
		result, err := s.execInstall(ctx, execution, sessionID,
			"if test -f "+sandbox.ShellQuote(marker)+"; then cat "+sandbox.ShellQuote(marker)+"; fi")
		if err != nil {
			return err
		}
		if strings.TrimSpace(result.Stdout) == row.BundleSHA256 {
			return nil
		}
		archive, err := s.skillBundleArchive(ctx, configID, row.ID)
		if err != nil {
			return err
		}
		if !archiveMatchesSHA(archive, row.BundleSHA256) {
			return fmt.Errorf("skill bundle digest mismatch")
		}
		bundle, err := ParseSkillBundle(archive)
		if err != nil {
			return err
		}
		if err := s.resetSkillDir(ctx, execution, sessionID, dir); err != nil {
			return err
		}
		if err := s.seedSkillFiles(ctx, execution, sessionID, dir, bundle); err != nil {
			return err
		}
		if s.messages == nil {
			return fmt.Errorf("message repository is unavailable")
		}
		ownerID := strings.TrimSpace(sessionUserIDFromContext(ctx))
		if ownerID == "" {
			return fmt.Errorf("skill preparation requires a session owner")
		}
		logSession, err := s.sessions.Create(ctx, &types.Session{
			TenantID: tenantID, UserID: ownerID,
			Title:           "Skill preparation",
			Description:     types.SkillMaintenanceSessionMarker + "prepare",
			SandboxConfigID: configID,
		})
		if err != nil {
			return err
		}
		prompt := buildInstallPrompt(dir, bundle, s.probeInstallTools(ctx, execution, sessionID))
		transcript := newSessionInstallTranscript(
			ctx, event.NewEventBus(), s.streams, s.messages, logSession.ID, uuid.NewString(),
		)
		if err := transcript.Create(ctx, prompt); err != nil {
			return err
		}
		transcript.Subscribe()
		if err := s.installDependenciesAndVerify(ctx, installerJob{
			configID: configID, skillID: row.ID, executionID: sessionID,
			mgr: execution, transcript: transcript, prompt: prompt, sessionPreparation: true,
			skillDir: dir, bundle: bundle,
		}); err != nil {
			return fmt.Errorf("prepare skill %s: %w", row.Name, err)
		}
		_, err = s.execInstall(ctx, execution, sessionID,
			"printf %s "+sandbox.ShellQuote(row.BundleSHA256)+" > "+sandbox.ShellQuote(marker)+
				" && chmod 444 "+sandbox.ShellQuote(marker))
		return err
	})
}
