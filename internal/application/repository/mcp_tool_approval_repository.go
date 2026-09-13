package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MCPToolApprovalRepository implements interfaces.MCPToolApprovalRepository.
type MCPToolApprovalRepository struct {
	db *gorm.DB
}

// NewMCPToolApprovalRepository creates a repository backed by GORM.
func NewMCPToolApprovalRepository(db *gorm.DB) interfaces.MCPToolApprovalRepository {
	return &MCPToolApprovalRepository{db: db}
}

// ListByService returns all stored approval rows for an MCP service (may be empty).
func (r *MCPToolApprovalRepository) ListByService(ctx context.Context, serviceID string) ([]*types.MCPToolApproval, error) {
	var rows []*types.MCPToolApproval
	err := r.db.WithContext(ctx).
		Where("service_id = ?", serviceID).
		Order("tool_name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list mcp tool approvals: %w", err)
	}
	return rows, nil
}

// IsRequired returns true when a row exists with require_approval = true.
func (r *MCPToolApprovalRepository) IsRequired(ctx context.Context, serviceID, toolName string) (bool, error) {
	var row types.MCPToolApproval
	err := r.db.WithContext(ctx).
		Select("require_approval").
		Where("service_id = ? AND tool_name = ?", serviceID, toolName).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get mcp tool approval: %w", err)
	}
	return row.RequireApproval, nil
}

// Upsert creates or updates the approval flag for a tool atomically.
// Uses ON CONFLICT against the (service_id, tool_name) unique index
// so concurrent writers don't race the prior SELECT-then-INSERT path into
// duplicate-key 500s.
func (r *MCPToolApprovalRepository) Upsert(ctx context.Context, row *types.MCPToolApproval) error {
	if row == nil {
		return errors.New("row is nil")
	}
	if row.ID == "" {
		row.ID = uuid.New().String()
	}
	now := time.Now()
	row.UpdatedAt = now
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "service_id"},
			{Name: "tool_name"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"require_approval": row.RequireApproval,
			"updated_at":       now,
		}),
	}).Create(row).Error
	if err != nil {
		return fmt.Errorf("upsert mcp tool approval: %w", err)
	}
	return nil
}
