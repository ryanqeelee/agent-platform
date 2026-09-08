package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *memoryRepository) WithAuthority(
	ctx context.Context,
	scope interfaces.MemoryScope,
	request interfaces.MemoryAuthorityRequest,
	callback interfaces.MemoryAuthorityCallback,
) (*types.PersonalMemoryReceipt, error) {
	if !scope.Valid() {
		return nil, errors.New("memory: invalid authority scope")
	}
	var result *types.PersonalMemoryReceipt
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		if err := tx.Select("id", "memory_config", "memory_generation").
			Where("id = ?", scope.TenantID).
			Clauses(clause.Locking{Strength: "SHARE"}).First(&tenant).Error; err != nil {
			return err
		}

		txRepo := &memoryRepository{db: tx}
		if _, err := txRepo.EnsureSubject(ctx, scope); err != nil {
			return err
		}
		var subject types.MemorySubject
		if err := tx.Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID).
			Clauses(forUpdateClause()).First(&subject).Error; err != nil {
			return err
		}

		if request.OperationID != "" {
			existing, err := txRepo.GetCommandReceipt(ctx, scope, request.OperationID)
			if err != nil {
				return err
			}
			if existing != nil {
				if existing.CommandHash != request.CommandHash {
					return interfaces.ErrMemoryOperationConflict
				}
				result = existing.DTO()
				return nil
			}
		}

		cfg := normalizedMemoryConfig(tenant.MemoryConfig)
		state := interfaces.MemoryAuthorityState{
			Config: cfg, WorkspaceGeneration: tenant.MemoryGeneration,
			SubjectGeneration: subject.Generation, Revision: subject.Revision,
			UserEnabled: subject.Enabled,
		}
		reason := request.PreRejectReason
		if reason == "" && request.RequireEnabled && (!cfg.MemoryEnabled() || !subject.Enabled) {
			reason = types.MemoryReasonPolicyDisabled
		} else if request.RequireAuto && !cfg.AutoExtractEnabled() {
			reason = types.MemoryReasonPolicyDisabled
		} else if request.Expected != nil &&
			(request.Expected.WorkspaceGeneration != state.WorkspaceGeneration ||
				request.Expected.SubjectGeneration != state.SubjectGeneration) {
			reason = types.MemoryReasonPolicyStale
		} else if request.Expected != nil && request.Expected.Revision != state.Revision {
			reason = types.MemoryReasonRevisionConflict
		}

		mutation := &interfaces.MemoryAuthorityMutation{}
		if reason == "" && callback != nil {
			var err error
			mutation, err = callback(ctx, txRepo, state)
			if err != nil {
				return err
			}
			if mutation == nil {
				mutation = &interfaces.MemoryAuthorityMutation{}
			}
			reason = mutation.ReasonCode
		}

		committedAt := (*time.Time)(nil)
		status := types.MemoryReceiptRejected
		if reason == "" {
			now := time.Now().UTC()
			committedAt = &now
			status = types.MemoryReceiptNoop
			if mutation.Mutated {
				status = types.MemoryReceiptApplied
			}
			updates := map[string]interface{}{}
			if mutation.BumpRevision {
				subject.Revision++
				updates["revision"] = subject.Revision
			}
			if request.BumpGeneration && mutation.Mutated {
				subject.Generation++
				updates["generation"] = subject.Generation
			}
			if len(updates) > 0 {
				updates["updated_at"] = now
				if err := tx.Model(&types.MemorySubject{}).
					Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID).
					Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		var reasonPtr *string
		if reason != "" {
			reasonCopy := reason
			reasonPtr = &reasonCopy
		}
		receipt := &types.MemoryCommandReceipt{
			ID: uuid.NewString(), TenantID: scope.TenantID, SubjectID: scope.SubjectID,
			OperationID: request.OperationID, CommandHash: request.CommandHash,
			Status: status, ReasonCode: reasonPtr, Revision: subject.Revision,
			WorkspaceGeneration: tenant.MemoryGeneration, SubjectGeneration: subject.Generation,
			ItemIDs: types.MemoryStringList(mutation.ItemIDs), CommittedAt: committedAt,
		}
		if request.OperationID != "" {
			if err := tx.Create(receipt).Error; err != nil {
				return err
			}
		}
		result = receipt.DTO()
		return nil
	})
	return result, err
}

func (r *memoryRepository) GetCommandReceipt(
	ctx context.Context, scope interfaces.MemoryScope, operationID string,
) (*types.MemoryCommandReceipt, error) {
	if operationID == "" {
		return nil, nil
	}
	var receipt types.MemoryCommandReceipt
	err := r.scoped(ctx, scope).Where("operation_id = ?", operationID).First(&receipt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &receipt, nil
}

func (r *memoryRepository) AcceptExpression(
	ctx context.Context, scope interfaces.MemoryScope, expression *types.MemoryExpression,
) (*types.PersonalMemoryExpressionReceipt, error) {
	if expression == nil || !scope.Valid() {
		return nil, errors.New("memory: invalid expression")
	}
	var receipt *types.PersonalMemoryExpressionReceipt
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		if err := tx.Select("id", "memory_config", "memory_generation").Where("id = ?", scope.TenantID).
			Clauses(clause.Locking{Strength: "SHARE"}).First(&tenant).Error; err != nil {
			return err
		}
		txRepo := &memoryRepository{db: tx}
		if _, err := txRepo.EnsureSubject(ctx, scope); err != nil {
			return err
		}
		var subject types.MemorySubject
		if err := tx.Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID).
			Clauses(forUpdateClause()).First(&subject).Error; err != nil {
			return err
		}
		var existing types.MemoryExpression
		err := tx.Where("tenant_id = ? AND subject_id = ? AND expression_id = ?",
			scope.TenantID, scope.SubjectID, expression.ExpressionID).First(&existing).Error
		if err == nil {
			if existing.ExpressionHash != expression.ExpressionHash {
				return interfaces.ErrMemoryOperationConflict
			}
			receipt = &types.PersonalMemoryExpressionReceipt{
				Schema:       types.PersonalMemoryExpressionReceiptSchema,
				ExpressionID: expression.ExpressionID, Status: types.MemoryExpressionReplayed,
				ReasonCode: existing.OutcomeReason, Pending: existing.Status == types.MemoryExpressionPending,
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		cfg := normalizedMemoryConfig(tenant.MemoryConfig)
		reason := ""
		if !cfg.AutoExtractEnabled() || !subject.Enabled {
			reason = types.MemoryReasonPolicyDisabled
		} else if expression.WorkspaceGeneration != tenant.MemoryGeneration ||
			expression.SubjectGeneration != subject.Generation {
			reason = types.MemoryReasonPolicyStale
		}
		if reason != "" {
			now := time.Now().UTC()
			expression.ID = uuid.NewString()
			expression.TenantID = scope.TenantID
			expression.SubjectID = scope.SubjectID
			expression.Text = nil
			expression.Status = types.MemoryExpressionProcessed
			expression.OutcomeReason = &reason
			expression.ProcessedAt = &now
			if err := tx.Create(expression).Error; err != nil {
				return err
			}
			receipt = &types.PersonalMemoryExpressionReceipt{
				Schema:       types.PersonalMemoryExpressionReceiptSchema,
				ExpressionID: expression.ExpressionID, Status: types.MemoryExpressionRejected,
				ReasonCode: &reason,
			}
			return nil
		}
		expression.ID = uuid.NewString()
		expression.TenantID = scope.TenantID
		expression.SubjectID = scope.SubjectID
		expression.Status = types.MemoryExpressionPending
		if err := tx.Create(expression).Error; err != nil {
			return err
		}
		receipt = &types.PersonalMemoryExpressionReceipt{
			Schema:       types.PersonalMemoryExpressionReceiptSchema,
			ExpressionID: expression.ExpressionID, Status: types.MemoryExpressionAccepted, Pending: true,
		}
		return nil
	})
	return receipt, err
}

func (r *memoryRepository) ListPendingExpressions(
	ctx context.Context, scope interfaces.MemoryScope, limit int,
) ([]*types.MemoryExpression, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []*types.MemoryExpression
	err := r.scoped(ctx, scope).Where("status = ?", types.MemoryExpressionPending).
		Order("created_at ASC, id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *memoryRepository) MarkExpressionsProcessed(
	ctx context.Context, scope interfaces.MemoryScope, expressionIDs []string, reason string,
) error {
	if len(expressionIDs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status": types.MemoryExpressionProcessed, "text": nil, "processed_at": now,
	}
	if reason != "" {
		updates["outcome_reason"] = reason
	}
	return r.scoped(ctx, scope).Model(&types.MemoryExpression{}).
		Where("expression_id IN ? AND status = ?", expressionIDs, types.MemoryExpressionPending).
		Updates(updates).Error
}
