package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

type SkillCatalogInstallView struct {
	SkillID           string    `json:"skill_id"`
	SandboxConfigID   string    `json:"sandbox_config_id"`
	SandboxConfigName string    `json:"sandbox_config_name,omitempty"`
	SandboxType       string    `json:"sandbox_type,omitempty"`
	Status            string    `json:"status"`
	Enabled           bool      `json:"enabled"`
	Error             string    `json:"error,omitempty"`
	BundleSHA256      string    `json:"bundle_sha256,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SkillCatalogView struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Version       string                    `json:"version,omitempty"`
	Description   string                    `json:"description,omitempty"`
	BundleSHA256  string                    `json:"bundle_sha256,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Installations []SkillCatalogInstallView `json:"installations"`
}

func (s *TenantSkillService) ListCatalog(ctx context.Context) ([]SkillCatalogView, error) {
	catalogs, err := s.skills.ListCatalogs(ctx)
	if err != nil {
		return nil, err
	}
	installs, err := s.skills.ListSkills(ctx)
	if err != nil {
		return nil, err
	}
	configs, err := s.configs.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	configByID := make(map[string]*types.TenantSandboxConfigEntity, len(configs))
	for _, cfg := range configs {
		if cfg != nil {
			configByID[cfg.ID] = cfg
		}
	}
	byCatalog := make(map[string][]*types.TenantSkillEntity, len(catalogs))
	for _, row := range installs {
		if row != nil && row.CatalogID != "" {
			byCatalog[row.CatalogID] = append(byCatalog[row.CatalogID], row)
		}
	}
	out := make([]SkillCatalogView, 0, len(catalogs))
	for _, cat := range catalogs {
		if cat != nil {
			out = append(out, catalogView(cat, byCatalog[cat.ID], configByID))
		}
	}
	return out, nil
}

func catalogView(cat *types.TenantSkillCatalogEntity, installs []*types.TenantSkillEntity,
	configByID map[string]*types.TenantSandboxConfigEntity) SkillCatalogView {
	view := SkillCatalogView{
		ID: cat.ID, Name: cat.Name, Version: cat.Version,
		Description: cat.Description, BundleSHA256: cat.BundleSHA256,
		CreatedAt: cat.CreatedAt, UpdatedAt: cat.UpdatedAt,
		Installations: make([]SkillCatalogInstallView, 0, len(installs)),
	}
	for _, row := range installs {
		view.Installations = append(view.Installations, installView(row, configByID))
	}
	return view
}

func installView(row *types.TenantSkillEntity,
	configByID map[string]*types.TenantSandboxConfigEntity) SkillCatalogInstallView {
	view := SkillCatalogInstallView{
		SkillID: row.ID, SandboxConfigID: row.SandboxConfigID,
		Status: row.Status, Enabled: row.Enabled, Error: row.Error,
		BundleSHA256: row.BundleSHA256, UpdatedAt: row.UpdatedAt,
	}
	if cfg := configByID[row.SandboxConfigID]; cfg != nil {
		view.SandboxConfigName, view.SandboxType = cfg.Name, cfg.SandboxType
	}
	return view
}

func (s *TenantSkillService) RegisterCatalogFromArchive(
	ctx context.Context, archive []byte,
) (*types.TenantSkillCatalogEntity, error) {
	bundle, err := ParseSkillBundle(archive)
	if err != nil {
		return nil, err
	}
	return s.createCatalogFromBundle(ctx, bundle, archive)
}

func (s *TenantSkillService) RegisterCatalogFromSource(
	ctx context.Context, source string,
) (*types.TenantSkillCatalogEntity, error) {
	bundle, archive, err := fetchNormalizedSkillBundle(ctx, source, s.sourceHTTP)
	if err != nil {
		return nil, err
	}
	return s.createCatalogFromBundle(ctx, bundle, archive)
}

func (s *TenantSkillService) createCatalogFromBundle(
	ctx context.Context, bundle *SkillBundle, archive []byte,
) (*types.TenantSkillCatalogEntity, error) {
	if s.archives == nil {
		return nil, errors.New("platform skill archive store is unavailable")
	}
	now := s.clock()()
	row := &types.TenantSkillCatalogEntity{
		ID: uuid.NewString(), Name: bundle.Name, Version: bundle.Version,
		Description: bundle.Description, Instructions: bundle.Instructions,
		BundleSHA256: bundle.SHA256, CreatedAt: now, UpdatedAt: now,
	}
	ref, err := s.archives.Put(ctx, fmt.Sprintf("catalog/%s.zip", row.ID), archive)
	if err != nil {
		return nil, fmt.Errorf("store platform skill archive: %w", err)
	}
	row.BundleRef = ref
	if err := s.skills.CreateCatalog(ctx, row); err != nil {
		if deleteErr := s.archives.Delete(context.WithoutCancel(ctx), ref); deleteErr != nil {
			logger.Warnf(ctx, "[skill] delete unreferenced archive %s: %v", ref, deleteErr)
		}
		return nil, err
	}
	return row, nil
}

type CatalogInstallResult struct {
	Installs map[string]string `json:"installs"`
	Errors   map[string]string `json:"errors,omitempty"`
}

func (s *TenantSkillService) InstallCatalogToConfigs(
	ctx context.Context, catalogID string, configIDs []string,
) (*CatalogInstallResult, error) {
	catalog, err := s.resolveCatalog(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	archive, err := s.catalogBundleArchive(ctx, catalog)
	if err != nil {
		return nil, err
	}
	bundle, err := ParseSkillBundle(archive)
	if err != nil {
		return nil, err
	}
	ids := uniqueNonEmptyStrings(configIDs)
	if len(ids) == 0 {
		return nil, apperrors.NewBadRequestError("at least one sandbox is required")
	}
	result := &CatalogInstallResult{Installs: map[string]string{}, Errors: map[string]string{}}
	var firstErr error
	for _, configID := range ids {
		cfg, configErr := s.configs.GetByID(ctx, configID)
		if configErr != nil {
			result.Errors[configID] = skillUserErrorMessage(configErr)
			if firstErr == nil {
				firstErr = configErr
			}
			continue
		}
		if cfg == nil {
			installErr := apperrors.NewNotFoundError("sandbox config not found")
			result.Errors[configID] = skillUserErrorMessage(installErr)
			if firstErr == nil {
				firstErr = installErr
			}
			continue
		}
		skillID, installErr := s.installParsedSkill(ctx, configID, bundle, archive, catalog)
		if installErr != nil {
			result.Errors[configID] = skillUserErrorMessage(installErr)
			if firstErr == nil {
				firstErr = installErr
			}
			continue
		}
		result.Installs[configID] = skillID
	}
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	if len(result.Installs) == 0 {
		return result, firstErr
	}
	return result, nil
}

func (s *TenantSkillService) DeleteCatalog(ctx context.Context, catalogID string) error {
	catalog, err := s.skills.GetCatalog(ctx, catalogID)
	if err != nil {
		return err
	}
	if catalog == nil {
		return apperrors.NewNotFoundError("skill not found")
	}
	installs, err := s.skills.ListSkillsByCatalog(ctx, catalogID)
	if err != nil {
		return err
	}
	if len(installs) != 0 {
		return apperrors.NewConflictError("remove this skill from every sandbox before deleting it from the catalog")
	}
	if err := s.skills.DeleteCatalog(ctx, catalogID); err != nil {
		return err
	}
	if catalog.BundleRef != "" && s.archives != nil {
		if err := s.archives.Delete(ctx, catalog.BundleRef); err != nil {
			logger.Warnf(ctx, "[skill] delete platform archive %s: %v", catalog.BundleRef, err)
		}
	}
	return nil
}

func skillUserErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var app *apperrors.AppError
	if errors.As(err, &app) && strings.TrimSpace(app.Message) != "" {
		return app.Message
	}
	return err.Error()
}

func (s *TenantSkillService) resolveCatalog(
	ctx context.Context, id string,
) (*types.TenantSkillCatalogEntity, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.NewNotFoundError("skill not found")
	}
	cat, err := s.skills.GetCatalog(ctx, id)
	if err != nil {
		return nil, err
	}
	if cat == nil {
		return nil, apperrors.NewNotFoundError("skill not found")
	}
	return cat, nil
}

func (s *TenantSkillService) ListCatalogFiles(
	ctx context.Context, catalogID string,
) ([]SkillFileEntry, error) {
	archive, err := s.loadCatalogArchive(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	return listSkillZipFiles(archive)
}

func (s *TenantSkillService) ReadCatalogFile(
	ctx context.Context, catalogID, relativePath string,
) (*SkillFileContent, error) {
	clean, err := safeSkillFilePath(relativePath)
	if err != nil {
		return nil, apperrors.NewBadRequestError(err.Error())
	}
	archive, err := s.loadCatalogArchive(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	body, err := readSkillZipFile(archive, clean)
	if errors.Is(err, errSkillFileMissing) {
		return nil, apperrors.NewNotFoundError("skill file not found")
	}
	if err != nil {
		return nil, err
	}
	return projectSkillFileContent(clean, body), nil
}

func (s *TenantSkillService) loadCatalogArchive(
	ctx context.Context, catalogID string,
) ([]byte, error) {
	catalog, err := s.resolveCatalog(ctx, catalogID)
	if err != nil {
		return nil, err
	}
	return s.catalogBundleArchive(ctx, catalog)
}

func (s *TenantSkillService) catalogBundleArchive(
	ctx context.Context, catalog *types.TenantSkillCatalogEntity,
) ([]byte, error) {
	if catalog == nil || strings.TrimSpace(catalog.BundleRef) == "" || s.archives == nil {
		return nil, apperrors.NewBadRequestError("the platform skill archive is unavailable")
	}
	reader, err := s.archives.Open(ctx, catalog.BundleRef)
	if err != nil {
		return nil, fmt.Errorf("open platform skill archive: %w", err)
	}
	defer reader.Close()
	archive, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if want := strings.TrimSpace(catalog.BundleSHA256); want != "" && !archiveMatchesSHA(archive, want) {
		return nil, fmt.Errorf("platform skill archive digest mismatch")
	}
	return archive, nil
}
