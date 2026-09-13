package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TenantSkillRepository persists the platform-global skill catalog,
// installations and snapshot ledger. User environment
// values remain tenant scoped in the same concrete repository but are never
// deleted as a side effect of platform maintenance.
type TenantSkillRepository interface {
	// CreateSkill inserts a metadata projection row before provider-side image work starts.
	CreateSkill(ctx context.Context, e *types.TenantSkillEntity) error
	// GetSkill returns nil (no error) when the skill does not exist or belongs
	// to another workspace/config, so callers can render a 404 directly.
	GetSkill(ctx context.Context, configID, skillID string) (*types.TenantSkillEntity, error)
	// GetSkillByName scopes lookup by config because skill names are only unique within a config.
	GetSkillByName(ctx context.Context, configID, name string) (*types.TenantSkillEntity, error)
	// ListSkillsByConfig returns the installed skill projection for one sandbox config.
	ListSkillsByConfig(ctx context.Context, configID string) ([]*types.TenantSkillEntity, error)
	ListSkills(ctx context.Context) ([]*types.TenantSkillEntity, error)
	// UpdateSkill writes the mutable projection fields after install/remove
	// state changes. It never touches admin-owned envs/enabled or run-owned
	// transcript/token fields; see the implementation.
	UpdateSkill(ctx context.Context, e *types.TenantSkillEntity) error
	// BeginSkillRun atomically hands the row to a new install/remove token and
	// clears the previous install transcript.
	BeginSkillRun(
		ctx context.Context, configID, skillID, runID, status string, startedAt time.Time,
	) error
	// UpdateSkillForRun updates lifecycle fields only while runID still owns
	// the row. matched=false means a newer operation took ownership.
	UpdateSkillForRun(
		ctx context.Context, e *types.TenantSkillEntity, runID string,
	) (matched bool, err error)
	// UpdateInstallTranscript persists the latest prompt/assistant projection
	// only while the supplied install token remains current.
	UpdateInstallTranscript(
		ctx context.Context, configID, skillID, runID string, transcript types.JSON,
	) (matched bool, err error)
	// UpdateSkillEnvs writes the declared environment variables alone.
	UpdateSkillEnvs(
		ctx context.Context, configID, skillID string, envs types.SkillEnvVars,
	) error
	UpdateSkillEnvsForRun(
		ctx context.Context, configID, skillID, runID string, envs types.SkillEnvVars,
	) (matched bool, err error)
	// UpdateSkillAdminState writes visibility and declared values together, as
	// one admin request.
	UpdateSkillAdminState(
		ctx context.Context,
		configID, skillID string,
		enabled bool,
		envs types.SkillEnvVars,
	) error
	// DeleteSkill soft-deletes only the platform metadata row. Tenant-owned
	// per-principal environment values are never deleted by platform maintenance.
	DeleteSkill(ctx context.Context, configID, skillID string) error
	DeleteSkillForRun(
		ctx context.Context, configID, skillID, runID string,
	) (matched bool, err error)
	// ListStaleInstalling finds abandoned install/remove runs for the reaper.
	ListStaleInstalling(ctx context.Context, olderThan time.Time) ([]*types.TenantSkillEntity, error)

	// CreateSnapshotRow records provider work before creating the billable snapshot.
	CreateSnapshotRow(ctx context.Context, e *types.TenantSkillSnapshotEntity) error
	// MarkSnapshotState updates ledger state and stores the provider snapshot ID once known.
	MarkSnapshotState(ctx context.Context, id, state, snapshotID string) error
	// ListSnapshotsByConfig returns the full chain for audit and troubleshooting.
	ListSnapshotsByConfig(ctx context.Context, configID string) ([]*types.TenantSkillSnapshotEntity, error)
	// DeleteSnapshotRowsByConfig removes ledger rows only when an entire sandbox
	// config is deleted and its provider-side snapshots are already gone; never
	// call this during an ordinary image switch (old snapshots stay in the ledger).
	DeleteSnapshotRowsByConfig(ctx context.Context, configID string) error
	CountUserEnvVarsByConfig(ctx context.Context, configID string) (int64, error)

	// ListUserEnvVars returns one principal's own values for one scope,
	// decrypted. An empty skillID selects the config-wide variables.
	ListUserEnvVars(
		ctx context.Context, tenantID uint64, p types.Principal, configID, skillID string,
	) ([]*types.TenantUserEnvVar, error)
	// ListUserEnvVarsByConfig returns every scope at once, for the settings
	// page that renders a whole config in one go.
	ListUserEnvVarsByConfig(
		ctx context.Context, tenantID uint64, p types.Principal, configID string,
	) ([]*types.TenantUserEnvVar, error)
	// UpsertUserEnvVar writes a principal's value, replacing any previous one
	// for the same (tenant, principal, config, skill, name).
	UpsertUserEnvVar(ctx context.Context, e *types.TenantUserEnvVar) error
	// DeleteUserEnvVar removes one value, returning types.ErrEnvVarNotFound
	// when there was nothing to remove.
	DeleteUserEnvVar(
		ctx context.Context, tenantID uint64, p types.Principal, configID, skillID, name string,
	) error
	// CreateCatalog inserts a platform skill definition.
	CreateCatalog(ctx context.Context, e *types.TenantSkillCatalogEntity) error
	GetCatalog(ctx context.Context, catalogID string) (*types.TenantSkillCatalogEntity, error)
	GetCatalogByName(ctx context.Context, name string) (*types.TenantSkillCatalogEntity, error)
	ListCatalogs(ctx context.Context) ([]*types.TenantSkillCatalogEntity, error)
	// UpdateCatalog writes mutable definition fields (bundle, description, version).
	UpdateCatalog(ctx context.Context, e *types.TenantSkillCatalogEntity) error
	// DeleteCatalog soft-deletes a definition. Install rows are not touched.
	DeleteCatalog(ctx context.Context, catalogID string) error
	// ListSkillsByCatalog returns installations of one catalog skill.
	ListSkillsByCatalog(ctx context.Context, catalogID string) ([]*types.TenantSkillEntity, error)
}

type tenantSkillRepository struct{ db *gorm.DB }

// NewTenantSkillRepository returns a GORM-backed implementation.
func NewTenantSkillRepository(db *gorm.DB) TenantSkillRepository {
	return &tenantSkillRepository{db: db}
}

func (r *tenantSkillRepository) CreateSkill(ctx context.Context, e *types.TenantSkillEntity) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *tenantSkillRepository) GetSkill(
	ctx context.Context, configID, skillID string,
) (*types.TenantSkillEntity, error) {
	var e types.TenantSkillEntity
	err := r.db.WithContext(ctx).
		Where("sandbox_config_id = ? AND id = ?", configID, skillID).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *tenantSkillRepository) GetSkillByName(
	ctx context.Context, configID, name string,
) (*types.TenantSkillEntity, error) {
	var e types.TenantSkillEntity
	err := r.db.WithContext(ctx).
		Where("sandbox_config_id = ? AND name = ?", configID, name).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *tenantSkillRepository) ListSkillsByConfig(
	ctx context.Context, configID string,
) ([]*types.TenantSkillEntity, error) {
	var list []*types.TenantSkillEntity
	err := r.db.WithContext(ctx).
		Where("sandbox_config_id = ?", configID).
		Order("created_at ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (r *tenantSkillRepository) ListSkills(
	ctx context.Context,
) ([]*types.TenantSkillEntity, error) {
	var list []*types.TenantSkillEntity
	err := r.db.WithContext(ctx).
		Order("created_at ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateSkill writes the mutable columns explicitly so a zero-valued field on
// the passed entity cannot silently wipe state written by a concurrent job.
//
// envs is deliberately absent: it is the one column an install-progress write
// has no opinion about, and every such write is a read-modify-write of a row
// the install heartbeat reloads every 30 seconds. Including it would let a
// heartbeat that read the row a moment before a declaration was recorded put
// the stale list — or a NULL — back. UpdateSkillEnvs and UpdateSkillAdminState
// are the only writers of that column.
func skillProjectionUpdates(e *types.TenantSkillEntity) map[string]any {
	return map[string]any{
		"name":                  e.Name,
		"version":               e.Version,
		"description":           e.Description,
		"instructions":          e.Instructions,
		"bundle_ref":            e.BundleRef,
		"bundle_sha256":         e.BundleSHA256,
		"installed_snapshot_id": e.InstalledSnapshotID,
		"catalog_id":            e.CatalogID,
		"status":                e.Status,
		"error":                 e.Error,
		"installing_since":      e.InstallingSince,
		"updated_at":            time.Now(),
	}
}

func (r *tenantSkillRepository) UpdateSkill(ctx context.Context, e *types.TenantSkillEntity) error {
	return r.db.WithContext(ctx).
		Model(&types.TenantSkillEntity{}).
		Where("sandbox_config_id = ? AND id = ?", e.SandboxConfigID, e.ID).
		Updates(skillProjectionUpdates(e)).Error
}

func (r *tenantSkillRepository) BeginSkillRun(
	ctx context.Context, configID, skillID, runID, status string, startedAt time.Time,
) error {
	updates := map[string]any{
		"install_run_id":   runID,
		"status":           status,
		"error":            "",
		"installing_since": &startedAt,
		"updated_at":       time.Now(),
	}
	// A new install owns a new transcript. Removal still needs the same
	// compare-and-set token for terminal writes, but retains the last install
	// history if the image operation fails and the skill is restored.
	if status == types.SkillStatusInstalling {
		updates["install_transcript"] = nil
	}
	return r.db.WithContext(ctx).
		Model(&types.TenantSkillEntity{}).
		Where("sandbox_config_id = ? AND id = ?", configID, skillID).
		Updates(updates).Error
}

func (r *tenantSkillRepository) UpdateSkillForRun(
	ctx context.Context, e *types.TenantSkillEntity, runID string,
) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&types.TenantSkillEntity{}).
		Where("sandbox_config_id = ? AND id = ? AND install_run_id = ?",
			e.SandboxConfigID, e.ID, runID).
		Updates(skillProjectionUpdates(e))
	return result.RowsAffected == 1, result.Error
}

func (r *tenantSkillRepository) UpdateInstallTranscript(
	ctx context.Context, configID, skillID, runID string, transcript types.JSON,
) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&types.TenantSkillEntity{}).
		Where("sandbox_config_id = ? AND id = ? AND install_run_id = ?",
			configID, skillID, runID).
		Updates(map[string]any{
			"install_transcript": transcript,
			"updated_at":         time.Now(),
		})
	return result.RowsAffected == 1, result.Error
}

// UpdateSkillEnvs writes the declaration column alone, so recording what a
// skill needs cannot disturb the install state written next to it.
func (r *tenantSkillRepository) UpdateSkillEnvs(
	ctx context.Context, configID, skillID string, envs types.SkillEnvVars,
) error {
	return r.db.WithContext(ctx).
		Model(&types.TenantSkillEntity{}).
		Where("sandbox_config_id = ? AND id = ?", configID, skillID).
		Updates(map[string]any{"envs": envs, "updated_at": time.Now()}).Error
}

func (r *tenantSkillRepository) UpdateSkillEnvsForRun(
	ctx context.Context, configID, skillID, runID string, envs types.SkillEnvVars,
) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&types.TenantSkillEntity{}).
		Where("sandbox_config_id = ? AND id = ? AND install_run_id = ?",
			configID, skillID, runID).
		Updates(map[string]any{"envs": envs, "updated_at": time.Now()})
	return result.RowsAffected == 1, result.Error
}

// UpdateSkillAdminState writes the two columns an admin request owns, together,
// so a rotation that disables the skill and clears its value is one statement
// rather than two that can half apply.
func (r *tenantSkillRepository) UpdateSkillAdminState(
	ctx context.Context,
	configID, skillID string,
	enabled bool,
	envs types.SkillEnvVars,
) error {
	return r.db.WithContext(ctx).
		Model(&types.TenantSkillEntity{}).
		Where("sandbox_config_id = ? AND id = ?", configID, skillID).
		Updates(map[string]any{
			"enabled":    enabled,
			"envs":       envs,
			"updated_at": time.Now(),
		}).Error
}

// DeleteSkill soft-deletes only the platform installation projection. Tenant
// user values are business data and are not deleted by platform maintenance.
func (r *tenantSkillRepository) DeleteSkill(
	ctx context.Context, configID, skillID string,
) error {
	return r.db.WithContext(ctx).
		Where("sandbox_config_id = ? AND id = ?", configID, skillID).
		Delete(&types.TenantSkillEntity{}).Error
}

func (r *tenantSkillRepository) DeleteSkillForRun(
	ctx context.Context, configID, skillID, runID string,
) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("sandbox_config_id = ? AND id = ? AND install_run_id = ?",
			configID, skillID, runID).
		Delete(&types.TenantSkillEntity{})
	return result.RowsAffected == 1, result.Error
}

func (r *tenantSkillRepository) ListStaleInstalling(
	ctx context.Context, olderThan time.Time,
) ([]*types.TenantSkillEntity, error) {
	var list []*types.TenantSkillEntity
	err := r.db.WithContext(ctx).
		Where("status IN ? AND installing_since IS NOT NULL AND installing_since < ?",
			[]string{types.SkillStatusInstalling, types.SkillStatusRemoving}, olderThan).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (r *tenantSkillRepository) CreateSnapshotRow(
	ctx context.Context, e *types.TenantSkillSnapshotEntity,
) error {
	return r.db.WithContext(ctx).Create(e).Error
}

// MarkSnapshotState moves a ledger row and, when the snapshot has just been
// created, records its provider-side ID.
func (r *tenantSkillRepository) MarkSnapshotState(
	ctx context.Context, id, state, snapshotID string,
) error {
	updates := map[string]any{"state": state, "updated_at": time.Now()}
	if snapshotID != "" {
		updates["snapshot_id"] = snapshotID
	}
	if state == types.SkillSnapshotStateSuperseded {
		now := time.Now()
		updates["superseded_at"] = &now
	}
	return r.db.WithContext(ctx).
		Model(&types.TenantSkillSnapshotEntity{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (r *tenantSkillRepository) ListSnapshotsByConfig(
	ctx context.Context, configID string,
) ([]*types.TenantSkillSnapshotEntity, error) {
	var list []*types.TenantSkillSnapshotEntity
	err := r.db.WithContext(ctx).
		Where("sandbox_config_id = ?", configID).
		Order("generation ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteSnapshotRowsByConfig removes all ledger rows for a sandbox config.
// This is only legitimate when the entire sandbox config is being deleted and
// its provider-side snapshots have already been destroyed. During an ordinary
// image switch, old snapshots are never deleted—the ledger records their IDs
// and deleting rows would leave those IDs dangling. Here the config itself no
// longer exists, so keeping rows would point at a deleted config instead.
func (r *tenantSkillRepository) DeleteSnapshotRowsByConfig(
	ctx context.Context, configID string,
) error {
	return r.db.WithContext(ctx).
		Where("sandbox_config_id = ?", configID).
		Delete(&types.TenantSkillSnapshotEntity{}).Error
}

func (r *tenantSkillRepository) CountUserEnvVarsByConfig(
	ctx context.Context, configID string,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.TenantUserEnvVar{}).
		Where("sandbox_config_id = ?", configID).Count(&count).Error
	return count, err
}

func (r *tenantSkillRepository) ListUserEnvVars(
	ctx context.Context, tenantID uint64, p types.Principal, configID, skillID string,
) ([]*types.TenantUserEnvVar, error) {
	p = p.Normalize()
	var list []*types.TenantUserEnvVar
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND principal_type = ? AND principal_id = ?"+
			" AND sandbox_config_id = ? AND skill_id = ?",
			tenantID, p.Type, p.ID, configID, skillID).
		Order("name ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (r *tenantSkillRepository) ListUserEnvVarsByConfig(
	ctx context.Context, tenantID uint64, p types.Principal, configID string,
) ([]*types.TenantUserEnvVar, error) {
	p = p.Normalize()
	var list []*types.TenantUserEnvVar
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND principal_type = ? AND principal_id = ? AND sandbox_config_id = ?",
			tenantID, p.Type, p.ID, configID).
		Order("skill_id ASC, name ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// UpsertUserEnvVar conflicts on the unique index columns so a re-entered value
// replaces the previous one instead of accumulating rows.
func (r *tenantSkillRepository) UpsertUserEnvVar(
	ctx context.Context, e *types.TenantUserEnvVar,
) error {
	// Persist a copy: BeforeSave encrypts the receiver in place, and the
	// caller keeps holding the plaintext it passed in.
	stored := *e
	p := types.Principal{Type: stored.PrincipalType, ID: stored.PrincipalID}.Normalize()
	stored.PrincipalType, stored.PrincipalID = p.Type, p.ID
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"}, {Name: "principal_type"}, {Name: "principal_id"},
				{Name: "sandbox_config_id"}, {Name: "skill_id"}, {Name: "name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).
		Create(&stored).Error
}

func (r *tenantSkillRepository) DeleteUserEnvVar(
	ctx context.Context, tenantID uint64, p types.Principal, configID, skillID, name string,
) error {
	p = p.Normalize()
	res := r.db.WithContext(ctx).
		Where("tenant_id = ? AND principal_type = ? AND principal_id = ?"+
			" AND sandbox_config_id = ? AND skill_id = ? AND name = ?",
			tenantID, p.Type, p.ID, configID, skillID, name).
		Delete(&types.TenantUserEnvVar{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return types.ErrEnvVarNotFound
	}
	return nil
}

func (r *tenantSkillRepository) CreateCatalog(ctx context.Context, e *types.TenantSkillCatalogEntity) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *tenantSkillRepository) GetCatalog(
	ctx context.Context, catalogID string,
) (*types.TenantSkillCatalogEntity, error) {
	var e types.TenantSkillCatalogEntity
	err := r.db.WithContext(ctx).
		Where("id = ?", catalogID).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *tenantSkillRepository) GetCatalogByName(
	ctx context.Context, name string,
) (*types.TenantSkillCatalogEntity, error) {
	var e types.TenantSkillCatalogEntity
	err := r.db.WithContext(ctx).
		Where("name = ?", name).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *tenantSkillRepository) ListCatalogs(
	ctx context.Context,
) ([]*types.TenantSkillCatalogEntity, error) {
	var list []*types.TenantSkillCatalogEntity
	err := r.db.WithContext(ctx).
		Order("created_at ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (r *tenantSkillRepository) UpdateCatalog(ctx context.Context, e *types.TenantSkillCatalogEntity) error {
	return r.db.WithContext(ctx).
		Model(&types.TenantSkillCatalogEntity{}).
		Where("id = ?", e.ID).
		Updates(map[string]any{
			"name":          e.Name,
			"version":       e.Version,
			"description":   e.Description,
			"instructions":  e.Instructions,
			"bundle_ref":    e.BundleRef,
			"bundle_sha256": e.BundleSHA256,
			"updated_at":    time.Now(),
		}).Error
}

func (r *tenantSkillRepository) DeleteCatalog(ctx context.Context, catalogID string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", catalogID).
		Delete(&types.TenantSkillCatalogEntity{}).Error
}

func (r *tenantSkillRepository) ListSkillsByCatalog(
	ctx context.Context, catalogID string,
) ([]*types.TenantSkillEntity, error) {
	var list []*types.TenantSkillEntity
	err := r.db.WithContext(ctx).
		Where("catalog_id = ?", catalogID).
		Order("created_at ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}
