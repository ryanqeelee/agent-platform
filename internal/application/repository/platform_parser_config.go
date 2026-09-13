package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlatformParserConfigRepository persists the deployment singleton. Neither
// method accepts a tenant ID because tenant scope is not part of this authority.
type PlatformParserConfigRepository interface {
	Get(ctx context.Context) (*types.PlatformParserConfig, error)
	Upsert(ctx context.Context, config *types.ParserEngineConfig, actorID string, now time.Time) (*types.PlatformParserConfig, error)
}

type platformParserConfigRepository struct{ db *gorm.DB }

func NewPlatformParserConfigRepository(db *gorm.DB) PlatformParserConfigRepository {
	return &platformParserConfigRepository{db: db}
}

func (r *platformParserConfigRepository) Get(ctx context.Context) (*types.PlatformParserConfig, error) {
	var row types.PlatformParserConfig
	err := r.db.WithContext(ctx).
		Where("id = ?", types.PlatformParserConfigSingletonID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *platformParserConfigRepository) Upsert(
	ctx context.Context,
	config *types.ParserEngineConfig,
	actorID string,
	now time.Time,
) (*types.PlatformParserConfig, error) {
	row := &types.PlatformParserConfig{
		ID:        types.PlatformParserConfigSingletonID,
		Config:    config,
		UpdatedBy: actorID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"config", "updated_by", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return nil, err
	}
	return r.Get(ctx)
}
