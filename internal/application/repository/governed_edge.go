package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGovernedEdgeBindingConflict = errors.New("governed Edge binding conflict")
	ErrGovernedEnterpriseNotFound  = errors.New("governed enterprise not found")
)

func lockGovernedBindingTenant(ctx context.Context, tx *gorm.DB, predicate string, value any) (*types.Tenant, error) {
	var tenant types.Tenant
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "status", "analysis_enabled", "governed_enterprise_id", "governed_edge_binding").
		Where(predicate, value).Take(&tenant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGovernedEnterpriseNotFound
	}
	return &tenant, err
}

func writeGovernedBindingAudit(
	tx *gorm.DB,
	actorUserID string,
	tenantID uint64,
	action types.AuditAction,
	binding *types.GovernedEdgeBinding,
) error {
	details, err := json.Marshal(map[string]any{
		"binding_id": binding.BindingID, "enterprise_id": binding.EnterpriseID,
		"edge_node_id": binding.EdgeNodeID, "source_id": binding.SourceID,
		"revision": binding.Revision, "deployment_revision": binding.DeploymentRevision,
		"enabled": binding.Enabled,
	})
	if err != nil {
		return err
	}
	return tx.Create(&types.AuditLog{
		TenantID: tenantID, ActorUserID: actorUserID, ActorRole: "system_admin",
		Action: action, ScopeType: "tenant", ScopeID: fmt.Sprint(tenantID),
		TargetType: "governed_edge_binding", TargetID: binding.BindingID,
		Outcome: types.AuditOutcomeSuccess, Details: types.JSON(details),
	}).Error
}

func (r *tenantRepository) PrepareGovernedEdgeBinding(ctx context.Context, command interfaces.GovernedEdgeBindingPrepareCommand) (*types.GovernedEdgeBinding, error) {
	var result *types.GovernedEdgeBinding
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, command.ActorUserID); err != nil { return err }
		tenant, err := lockGovernedBindingTenant(ctx, tx, "id = ?", command.TenantID)
		if err != nil { return err }
		current := tenant.GovernedEdgeBinding
		if tenant.Status != types.TenantStatusActive || !tenant.AnalysisEnabled ||
			tenant.GovernedEnterpriseID == nil || current == nil || current.BindingID == "" ||
			current.EnterpriseID != *tenant.GovernedEnterpriseID {
			return ErrGovernedEdgeBindingConflict
		}
		exactCandidate := !current.Enabled && current.EdgeNodeID == command.EdgeNodeID &&
			current.SourceID == command.SourceID && current.DeploymentRevision == command.DeploymentRevision
		if exactCandidate && current.Revision == command.ExpectedRevision+1 {
			copy := *current; result = &copy; return nil
		}
		if current.Revision != command.ExpectedRevision ||
			(current.EdgeNodeID == command.EdgeNodeID && current.DeploymentRevision > command.DeploymentRevision) {
			return ErrGovernedEdgeBindingConflict
		}
		prepared := *current
		prepared.Revision++
		prepared.EdgeNodeID, prepared.SourceID = command.EdgeNodeID, command.SourceID
		prepared.DeploymentRevision, prepared.Enabled = command.DeploymentRevision, false
		if err := tx.Model(tenant).Update("governed_edge_binding", prepared).Error; err != nil { return err }
		if err := writeGovernedBindingAudit(tx, command.ActorUserID, tenant.ID, "ops.governed_edge_binding.prepared", &prepared); err != nil { return err }
		result = &prepared
		return nil
	})
	return result, err
}

func (r *tenantRepository) ConfirmGovernedEdgeBinding(ctx context.Context, command interfaces.GovernedEdgeBindingConfirmCommand) (*types.GovernedEdgeBinding, error) {
	var result *types.GovernedEdgeBinding
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, command.ActorUserID); err != nil { return err }
		tenant, err := lockGovernedBindingTenant(ctx, tx, "id = ?", command.TenantID)
		if err != nil { return err }
		current := tenant.GovernedEdgeBinding
		if tenant.Status != types.TenantStatusActive || !tenant.AnalysisEnabled || tenant.GovernedEnterpriseID == nil || current == nil || current.BindingID == "" || current.EnterpriseID != *tenant.GovernedEnterpriseID || current.EdgeNodeID != command.EdgeNodeID || current.SourceID != command.SourceID || current.DeploymentRevision != command.DeploymentRevision {
			return ErrGovernedEdgeBindingConflict
		}
		if current.Enabled && current.Revision == command.ExpectedRevision+1 {
			copy := *current; result = &copy; return nil
		}
		if current.Enabled || current.Revision != command.ExpectedRevision {
			return ErrGovernedEdgeBindingConflict
		}
		confirmed := *current
		confirmed.Revision++
		confirmed.Enabled = true
		if err := tx.Model(tenant).Update("governed_edge_binding", confirmed).Error; err != nil { return err }
		if err := writeGovernedBindingAudit(tx, command.ActorUserID, tenant.ID, "ops.governed_edge_binding.confirmed", &confirmed); err != nil { return err }
		result = &confirmed
		return nil
	})
	return result, err
}

func (r *tenantRepository) RevokeGovernedEdgeBinding(ctx context.Context, enterpriseID, edgeNodeID string, controlRevision int64) (*interfaces.EdgeNodeRevocationReceipt, error) {
	var receipt *interfaces.EdgeNodeRevocationReceipt
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tenant, err := lockGovernedBindingTenant(ctx, tx, "governed_enterprise_id = ?", enterpriseID)
		if err != nil { return err }
		current := tenant.GovernedEdgeBinding
		if current == nil || current.EnterpriseID != enterpriseID { return ErrGovernedEdgeBindingConflict }
		receipt = &interfaces.EdgeNodeRevocationReceipt{EnterpriseID: enterpriseID, EdgeNodeID: edgeNodeID, SentControlRevision: controlRevision, AcceptedControlRevision: current.DeploymentRevision, BindingRevision: current.Revision}
		if current.EdgeNodeID != edgeNodeID || current.DeploymentRevision > controlRevision {
			receipt.Status = "superseded"; return nil
		}
		if current.DeploymentRevision == controlRevision {
			if current.Enabled { return fmt.Errorf("%w: exact accepted deployment is enabled", ErrGovernedEdgeBindingConflict) }
			if current.SourceID == "" {
				receipt.Status = "disabled"
				return nil
			}
			revoked := *current
			revoked.Revision++
			revoked.SourceID = ""
			if err := tx.Model(tenant).Update("governed_edge_binding", revoked).Error; err != nil { return err }
			receipt.Status = "disabled"
			receipt.BindingRevision = revoked.Revision
			return nil
		}
		revoked := *current
		revoked.Revision++
		revoked.SourceID = ""
		revoked.DeploymentRevision, revoked.Enabled = controlRevision, false
		if err := tx.Model(tenant).Update("governed_edge_binding", revoked).Error; err != nil { return err }
		receipt.Status = "disabled"
		receipt.AcceptedControlRevision, receipt.BindingRevision = revoked.DeploymentRevision, revoked.Revision
		return nil
	})
	return receipt, err
}
