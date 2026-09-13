package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrOperatingBriefSnapshotNotFound = errors.New("operating brief snapshot not found")

type OperatingBriefRepository interface {
	CreateRefresh(context.Context, *types.OperatingBriefRefresh) (bool, error)
	MarkRefreshRunning(context.Context, string) error
	MarkRefreshFailed(context.Context, string, string) error
	SaveSnapshot(context.Context, string, *types.OperatingBriefSnapshot, map[string]string) error
	LatestSnapshot(context.Context, uint64, types.OperatingBriefScope) (*types.OperatingBriefSnapshot, error)
	LatestRefresh(context.Context, uint64, types.OperatingBriefScope) (*types.OperatingBriefRefresh, error)
	ResolveScope(context.Context, uint64, string) (*types.OperatingBriefScopeRef, error)
	ListScopes(context.Context, uint64) ([]types.OperatingBriefScopeRef, error)
	SnapshotQuestion(context.Context, uint64, string, string) (string, error)
}

type operatingBriefRepository struct{ db *gorm.DB }

func NewOperatingBriefRepository(db *gorm.DB) OperatingBriefRepository {
	return &operatingBriefRepository{db: db}
}

func (r *operatingBriefRepository) CreateRefresh(ctx context.Context, refresh *types.OperatingBriefRefresh) (bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "scope_kind"}, {Name: "store_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"id": refresh.ID, "requester_user_id": refresh.RequesterUserID,
			"status": refresh.Status, "error_code": "",
			"created_at": refresh.CreatedAt, "updated_at": refresh.UpdatedAt,
		}),
		Where: clause.Where{Exprs: []clause.Expression{clause.Expr{
			SQL: "operating_brief_refreshes.status NOT IN (?, ?)", Vars: []any{types.OperatingBriefRefreshQueued, types.OperatingBriefRefreshRunning},
		}}},
	}).Create(refresh)
	return result.RowsAffected == 1, result.Error
}

func (r *operatingBriefRepository) MarkRefreshRunning(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Model(&types.OperatingBriefRefresh{}).
		Where("id = ? AND status = ?", id, types.OperatingBriefRefreshQueued).
		Updates(map[string]any{"status": types.OperatingBriefRefreshRunning, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOperatingBriefSnapshotNotFound
	}
	return nil
}

func (r *operatingBriefRepository) MarkRefreshFailed(ctx context.Context, id, code string) error {
	return r.db.WithContext(ctx).Model(&types.OperatingBriefRefresh{}).
		Where("id = ? AND status IN ?", id, []string{types.OperatingBriefRefreshQueued, types.OperatingBriefRefreshRunning}).
		Updates(map[string]any{
			"status":     types.OperatingBriefRefreshFailed,
			"error_code": code,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (r *operatingBriefRepository) SaveSnapshot(
	ctx context.Context,
	refreshID string,
	snapshot *types.OperatingBriefSnapshot,
	allScopeLabels map[string]string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var refresh types.OperatingBriefRefresh
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", refreshID, types.OperatingBriefRefreshRunning)
		if err := query.Take(&refresh).Error; err != nil {
			return err
		}
		if refresh.TenantID != snapshot.TenantID || refresh.ScopeKind != snapshot.ScopeKind || refresh.StoreID != snapshot.StoreID {
			return errors.New("operating brief refresh scope changed")
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "scope_kind"}, {Name: "store_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"snapshot_ref", "source_id", "binding_id", "binding_revision", "deployment_revision",
				"catalog_version", "freshness_token", "state", "quality", "reason_code", "input_set_digest",
				"revision", "ready_at", "data_json", "handoff_questions_json", "snapshot_metadata_json",
				"fixed_slots_json", "generated_at",
			}),
		}).Create(snapshot).Error; err != nil {
			return err
		}
		if snapshot.ScopeKind == types.OperatingBriefScopeAll {
			now := snapshot.GeneratedAt
			ids := make([]string, 0, len(allScopeLabels))
			for storeID, label := range allScopeLabels {
				ids = append(ids, storeID)
				row := types.OperatingBriefScopeRef{
					ScopeRef: uuid.NewString(), TenantID: snapshot.TenantID,
					StoreID: storeID, Label: label, UpdatedAt: now,
				}
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "store_id"}},
					DoUpdates: clause.Assignments(map[string]any{"label": label, "updated_at": now}),
				}).Create(&row).Error; err != nil {
					return err
				}
			}
			query := tx.Where("tenant_id = ?", snapshot.TenantID)
			if len(ids) > 0 {
				query = query.Where("store_id NOT IN ?", ids)
			}
			if err := query.Delete(&types.OperatingBriefScopeRef{}).Error; err != nil {
				return err
			}
			for _, model := range []any{&types.OperatingBriefSnapshot{}, &types.OperatingBriefRefresh{}} {
				query := tx.Where("tenant_id = ? AND scope_kind = ?", snapshot.TenantID, types.OperatingBriefScopeStore)
				if len(ids) > 0 {
					query = query.Where("store_id NOT IN ?", ids)
				}
				if err := query.Delete(model).Error; err != nil {
					return err
				}
			}
		}
		result := tx.Model(&types.OperatingBriefRefresh{}).
			Where("id = ? AND status = ?", refreshID, types.OperatingBriefRefreshRunning).
			Updates(map[string]any{
				"status":     types.OperatingBriefRefreshSucceeded,
				"error_code": "",
				"updated_at": snapshot.GeneratedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOperatingBriefSnapshotNotFound
		}
		return nil
	})
}

func (r *operatingBriefRepository) LatestSnapshot(
	ctx context.Context, tenantID uint64, scope types.OperatingBriefScope,
) (*types.OperatingBriefSnapshot, error) {
	var snapshot types.OperatingBriefSnapshot
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND scope_kind = ? AND store_id = ?", tenantID, scope.Kind, scope.StoreID).
		Take(&snapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &snapshot, err
}

func (r *operatingBriefRepository) LatestRefresh(
	ctx context.Context, tenantID uint64, scope types.OperatingBriefScope,
) (*types.OperatingBriefRefresh, error) {
	var refresh types.OperatingBriefRefresh
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND scope_kind = ? AND store_id = ?", tenantID, scope.Kind, scope.StoreID).
		Take(&refresh).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &refresh, err
}

func (r *operatingBriefRepository) ResolveScope(ctx context.Context, tenantID uint64, ref string) (*types.OperatingBriefScopeRef, error) {
	var scope types.OperatingBriefScopeRef
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND scope_ref = ?", tenantID, ref).Take(&scope).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &scope, err
}

func (r *operatingBriefRepository) ListScopes(ctx context.Context, tenantID uint64) ([]types.OperatingBriefScopeRef, error) {
	var scopes []types.OperatingBriefScopeRef
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("store_id ASC").Find(&scopes).Error
	return scopes, err
}

func (r *operatingBriefRepository) SnapshotQuestion(
	ctx context.Context, tenantID uint64, snapshotRef, anchorRef string,
) (string, error) {
	var snapshot types.OperatingBriefSnapshot
	if err := r.db.WithContext(ctx).
		Select("handoff_questions_json").
		Where("tenant_id = ? AND snapshot_ref = ? AND state = ?", tenantID, snapshotRef, "ready").
		Take(&snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrOperatingBriefSnapshotNotFound
		}
		return "", err
	}
	var questions map[string]string
	if err := json.Unmarshal(snapshot.HandoffQuestions, &questions); err != nil {
		return "", err
	}
	question := questions[anchorRef]
	if question == "" {
		return "", ErrOperatingBriefSnapshotNotFound
	}
	return question, nil
}
