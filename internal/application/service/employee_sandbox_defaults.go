package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ConfigureEmployeeSandboxDefaults consumes operator-owned configuration only.
// Each enterprise receives its own encrypted named config and skill catalog.
func ConfigureEmployeeSandboxDefaults(agents interfaces.CustomAgentService,
	configs *TenantSandboxConfigService, skills *TenantSkillService,
) error {
	raw := strings.TrimSpace(os.Getenv("WEKNORA_EMPLOYEE_SANDBOX_DEFAULT_JSON"))
	if raw == "" {
		return nil
	}
	var template types.TenantSandboxConfig
	if json.Unmarshal([]byte(raw), &template) != nil {
		return fmt.Errorf("invalid employee sandbox default JSON")
	}
	if template.SkillPreparation != "session" {
		return fmt.Errorf("employee sandbox defaults require session skill preparation")
	}
	if _, err := SanitizeSandboxConfig(&template, nil); err != nil {
		return err
	}
	archives := make([]*SkillBundle, 0, 4)
	archiveBytes := make([][]byte, 0, 4)
	for _, name := range []string{"document-analyzer", "data-processor", "citation-generator", "doc-coauthoring"} {
		archive, err := builtinSkillArchive(filepath.Join("skills", "preloaded", name))
		if err != nil {
			return fmt.Errorf("prepare built-in skill %s: %w", name, err)
		}
		bundle, err := ParseSkillBundle(archive)
		if err != nil {
			return err
		}
		archives = append(archives, bundle)
		archiveBytes = append(archiveBytes, archive)
	}
	concrete, ok := agents.(*customAgentService)
	if !ok {
		return fmt.Errorf("employee sandbox provisioning requires the platform agent service")
	}
	concrete.provisionEmployeeSandbox = func(ctx context.Context, tenantID uint64) error {
		return skills.withConfigLock(ctx, tenantID, "employee-assistant-default", func(ctx context.Context) error {
			rows, err := configs.List(ctx, tenantID)
			if err != nil {
				return err
			}
			var created *types.TenantSandboxConfigEntity
			for _, row := range rows {
				if row.Name == "employee-assistant" {
					return nil
				}
				if row.Name == "employee-assistant-initializing" {
					created = row
				}
			}
			// Re-decode so config sanitization cannot mutate the shared template.
			var cfg types.TenantSandboxConfig
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return err
			}
			if created == nil {
				created, err = configs.Create(ctx, tenantID, CreateSandboxConfigInput{
					Name: "employee-assistant-initializing", Description: "企业员工助理统一执行环境", Config: &cfg,
				})
				if err != nil {
					return err
				}
			}
			// Keep a recoverable pending config until every bundle is accepted.
			// Once published, administrative removals are never auto-reversed.
			for i, bundle := range archives {
				existing, err := skills.skills.GetSkillByName(ctx, tenantID, created.ID, bundle.Name)
				if err != nil {
					return err
				}
				if existing != nil && (existing.Status == types.SkillStatusReady || existing.Status == types.SkillStatusInstalling) {
					continue
				}
				if _, err := skills.InstallSkill(ctx, tenantID, created.ID, archiveBytes[i]); err != nil {
					return fmt.Errorf("initialize employee skills: %w", err)
				}
			}
			_, err = configs.Update(ctx, tenantID, created.ID, UpdateSandboxConfigInput{
				Name: "employee-assistant", Description: created.Description,
			})
			return err
		})
	}
	return nil
}

func builtinSkillArchive(root string) ([]byte, error) {
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("built-in skill contains a symbolic link")
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		file, err := writer.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		_, err = file.Write(body)
		return err
	})
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
