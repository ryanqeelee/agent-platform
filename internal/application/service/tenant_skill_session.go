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
) error {
	cfg, err := s.configs.GetByID(ctx, tenantID, configID)
	if err != nil {
		return err
	}
	if cfg == nil || cfg.Config == nil || cfg.TenantID != tenantID {
		return fmt.Errorf("sandbox config is unavailable")
	}
	if cfg.Config.SkillPreparation != "session" {
		return nil
	}
	sess, err := s.sessions.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess == nil || sess.TenantID != tenantID {
		return fmt.Errorf("session is outside the workspace")
	}
	// All preparation in one session shares its seed archive and package tree.
	// Reuse the existing renewable lock implementation with a session namespace.
	return s.withConfigLock(ctx, tenantID, "session:"+sessionID, func(ctx context.Context) error {
		row, err := s.skills.GetSkill(ctx, tenantID, configID, selected.ID)
		if err != nil {
			return err
		}
		if row == nil || row.TenantID != tenantID || row.SandboxConfigID != configID || !row.Enabled || row.Status != types.SkillStatusReady ||
			row.BundleSHA256 != selected.BundleSHA256 || row.Name != selected.Name {
			return fmt.Errorf("skill changed or was disabled; retry with the current skill selection")
		}
		if len(row.BundleSHA256) != 64 {
			return fmt.Errorf("skill has no verified bundle digest")
		}
		dir, err := sandbox.SkillDirFor(row.Name)
		if err != nil {
			return err
		}
		marker := path.Join(dir, ".weknora-session-ready")
		result, err := s.execInstall(ctx, mgr, sessionID,
			"if test -f "+sandbox.ShellQuote(marker)+"; then cat "+sandbox.ShellQuote(marker)+"; fi")
		if err != nil {
			return err
		}
		if strings.TrimSpace(result.Stdout) == row.BundleSHA256 {
			return nil
		}
		archive, err := s.skillBundleArchive(ctx, tenantID, configID, row.ID)
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
		if err := s.resetSkillDir(ctx, mgr, sessionID, dir); err != nil {
			return err
		}
		if err := s.seedSkillFiles(ctx, mgr, sessionID, dir, bundle); err != nil {
			return err
		}
		// Keep installer messages in a separate, owner-scoped maintenance session.
		// Its engine targets the user's existing sandbox; it never receives user
		// environment secrets or a different enterprise's runtime.
		logSession, err := s.sessions.CreateSession(ctx, &types.Session{
			TenantID: tenantID, UserID: sessionUserIDFromContext(ctx),
			Title: "Skill preparation", Description: types.SkillMaintenanceSessionMarker + "prepare",
			SandboxConfigID: configID,
		})
		if err != nil {
			return err
		}
		prompt := buildInstallPrompt(dir, bundle, s.probeInstallTools(ctx, mgr, sessionID))
		transcript := newInstallTranscript(ctx, event.NewEventBus(), s.streams, s.messages,
			logSession.ID, uuid.NewString(), nil)
		if err := transcript.Create(ctx, prompt); err != nil {
			return err
		}
		transcript.Subscribe()
		runtimeSession := *sess
		runtimeSession.SandboxConfigID = configID
		if err := s.installDependenciesAndVerify(ctx, installerJob{
			tenantID: tenantID, configID: configID, skillID: row.ID,
			sess: &runtimeSession, mgr: mgr, transcript: transcript, prompt: prompt, sessionPreparation: true,
			skillDir: dir, bundle: bundle,
		}); err != nil {
			return fmt.Errorf("prepare skill %s: %w", row.Name, err)
		}
		_, err = s.execInstall(ctx, mgr, sessionID,
			"printf %s "+sandbox.ShellQuote(row.BundleSHA256)+" > "+sandbox.ShellQuote(marker)+
				" && chmod 444 "+sandbox.ShellQuote(marker))
		return err
	})
}
