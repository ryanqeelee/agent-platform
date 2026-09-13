package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCapabilityPlanNotFound   = errors.New("capability plan not found")
	ErrCapabilityTenantNotFound = errors.New("capability tenant not found")
)

// CapabilityPlanRepository is the local authority for immutable plans and
// scenario policy. Write methods recheck the durable system-admin identity.
type CapabilityPlanRepository interface {
	ValidateSystemAdministrator(context.Context, string) error
	CreatePlan(context.Context, string, *types.AICapabilityPlanVersion) error
	ListPlans(context.Context, string) ([]types.AICapabilityPlanVersion, *string, error)
	SetDefaultPlan(context.Context, string, string, time.Time) error
	AssignTenantPlan(context.Context, string, uint64, string, time.Time) error
	ClearTenantPlan(context.Context, string, uint64, time.Time) error
	ResolvePlan(context.Context, uint64) (*types.ResolvedAICapabilityPlan, error)
	GetDefaultScenario(context.Context, string) (types.AssistantScenarioCapabilities, error)
	SetDefaultScenario(context.Context, string, types.AssistantScenarioCapabilities, time.Time) error
	ResolveScenario(context.Context, uint64) (*types.ResolvedAssistantScenarioCapabilities, error)
	SetTenantScenario(context.Context, string, uint64, types.AssistantScenarioCapabilities, time.Time) error
	ClearTenantScenario(context.Context, string, uint64, time.Time) error
	GetTenant(context.Context, uint64) (*types.Tenant, error)
}

type capabilityPlanRepository struct{ db *gorm.DB }

// NewCapabilityPlanRepository constructs the local capability configuration store.
func NewCapabilityPlanRepository(db *gorm.DB) CapabilityPlanRepository {
	return &capabilityPlanRepository{db: db}
}

func writeCapabilityPlanAudit(
	tx *gorm.DB,
	actorID string,
	action types.AuditAction,
	targetType, targetID string,
	details map[string]any,
	now time.Time,
) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return tx.Create(&types.AuditLog{
		TenantID: 0, ActorUserID: actorID, ActorRole: "system_admin",
		Action: action, ScopeType: "system", ScopeID: "capability_configuration",
		TargetType: targetType, TargetID: targetID, Outcome: types.AuditOutcomeSuccess,
		Details: types.JSON(raw), CreatedAt: now,
	}).Error
}

func (r *capabilityPlanRepository) CreatePlan(
	ctx context.Context, actorID string, plan *types.AICapabilityPlanVersion,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorID); err != nil {
			return err
		}
		if err := tx.Create(plan).Error; err != nil {
			return err
		}
		return writeCapabilityPlanAudit(tx, actorID, "ops.ai_capability_plan.created", "ai_capability_plan", plan.VersionID,
			map[string]any{"scope": "version_registry", "version_id": plan.VersionID, "service_level": plan.ServiceLevel}, plan.CreatedAt)
	})
}

func (r *capabilityPlanRepository) requireSystemAdministrator(ctx context.Context, actorID string) error {
	return lockPlatformSystemAdministrator(ctx, r.db.WithContext(ctx), actorID)
}

// ValidateSystemAdministrator rechecks current durable authority for reads.
func (r *capabilityPlanRepository) ValidateSystemAdministrator(ctx context.Context, actorID string) error {
	return r.requireSystemAdministrator(ctx, actorID)
}

func (r *capabilityPlanRepository) ListPlans(
	ctx context.Context, actorID string,
) ([]types.AICapabilityPlanVersion, *string, error) {
	if err := r.requireSystemAdministrator(ctx, actorID); err != nil {
		return nil, nil, err
	}
	var plans []types.AICapabilityPlanVersion
	if err := r.db.WithContext(ctx).Order("created_at DESC, version_id ASC").Find(&plans).Error; err != nil {
		return nil, nil, err
	}
	var row types.AICapabilityPlanDefault
	err := r.db.WithContext(ctx).Where("singleton_key = ?", true).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return plans, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return plans, &row.VersionID, nil
}

func (r *capabilityPlanRepository) SetDefaultPlan(
	ctx context.Context, actorID, versionID string, now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorID); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&types.AICapabilityPlanVersion{}).Where("version_id = ?", versionID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrCapabilityPlanNotFound
		}
		row := types.AICapabilityPlanDefault{SingletonKey: true, VersionID: versionID, UpdatedBy: actorID, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "singleton_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"version_id", "updated_by", "updated_at"})}).Create(&row).Error; err != nil {
			return err
		}
		return writeCapabilityPlanAudit(tx, actorID, "ops.ai_capability_plan.default_set", "ai_capability_plan", versionID,
			map[string]any{"scope": "platform_shared", "version_id": versionID}, now)
	})
}

func (r *capabilityPlanRepository) AssignTenantPlan(
	ctx context.Context, actorID string, tenantID uint64, versionID string, now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorID); err != nil {
			return err
		}
		if err := requireCapabilityTenant(tx, tenantID); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&types.AICapabilityPlanVersion{}).Where("version_id = ?", versionID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrCapabilityPlanNotFound
		}
		row := types.TenantAICapabilityPlanAssignment{TenantID: tenantID, VersionID: versionID, UpdatedBy: actorID, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"version_id", "updated_by", "updated_at"})}).Create(&row).Error; err != nil {
			return err
		}
		return writeCapabilityPlanAudit(tx, actorID, "ops.ai_capability_plan.tenant_assigned", "tenant", formatTenantID(tenantID),
			map[string]any{"scope": "enterprise_assigned", "version_id": versionID}, now)
	})
}

func (r *capabilityPlanRepository) ClearTenantPlan(
	ctx context.Context, actorID string, tenantID uint64, now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorID); err != nil {
			return err
		}
		if err := requireCapabilityTenant(tx, tenantID); err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&types.TenantAICapabilityPlanAssignment{}).Error; err != nil {
			return err
		}
		return writeCapabilityPlanAudit(tx, actorID, "ops.ai_capability_plan.tenant_inherited", "tenant", formatTenantID(tenantID),
			map[string]any{"scope": "platform_shared"}, now)
	})
}

func requireCapabilityTenant(db *gorm.DB, tenantID uint64) error {
	if tenantID == 0 {
		return ErrCapabilityTenantNotFound
	}
	var count int64
	if err := db.Model(&types.Tenant{}).Where("id = ?", tenantID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrCapabilityTenantNotFound
	}
	return nil
}

func formatTenantID(tenantID uint64) string {
	return strconv.FormatUint(tenantID, 10)
}

func (r *capabilityPlanRepository) ResolvePlan(ctx context.Context, tenantID uint64) (*types.ResolvedAICapabilityPlan, error) {
	if tenantID == 0 {
		return nil, ErrCapabilityTenantNotFound
	}
	var row struct {
		types.AICapabilityPlanVersion
		ResolutionSource string `gorm:"column:resolution_source"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT p.*, CASE WHEN assignment.version_id IS NULL THEN 'platform_default' ELSE 'tenant_assignment' END AS resolution_source
		FROM tenants tenant
		LEFT JOIN tenant_ai_capability_plan_assignments assignment ON assignment.tenant_id = tenant.id
		LEFT JOIN ai_capability_plan_default default_plan ON default_plan.singleton_key = ?
		JOIN ai_capability_plan_versions p ON p.version_id = COALESCE(assignment.version_id, default_plan.version_id)
		WHERE tenant.id = ? AND tenant.deleted_at IS NULL`, true, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.VersionID == "" {
		return nil, nil
	}
	return &types.ResolvedAICapabilityPlan{Plan: row.AICapabilityPlanVersion, Source: row.ResolutionSource}, nil
}

func (r *capabilityPlanRepository) GetDefaultScenario(
	ctx context.Context, actorID string,
) (types.AssistantScenarioCapabilities, error) {
	if err := r.requireSystemAdministrator(ctx, actorID); err != nil {
		return types.AssistantScenarioCapabilities{}, err
	}
	var row types.AssistantScenarioCapabilityDefault
	err := r.db.WithContext(ctx).Where("singleton_key = ?", true).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.AssistantScenarioCapabilities{}, nil
	}
	if err != nil {
		return types.AssistantScenarioCapabilities{}, err
	}
	return scenarioCapabilities(row.ExternalSearch, row.MCP, row.Tools), nil
}

func (r *capabilityPlanRepository) SetDefaultScenario(
	ctx context.Context, actorID string, capabilities types.AssistantScenarioCapabilities, now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorID); err != nil {
			return err
		}
		row := types.AssistantScenarioCapabilityDefault{SingletonKey: true, ExternalSearch: capabilities.ExternalSearch,
			MCP: capabilities.MCP, Tools: capabilities.Tools, UpdatedBy: actorID, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "singleton_key"}}, DoUpdates: clause.AssignmentColumns(
			[]string{"external_search", "mcp", "tools", "updated_by", "updated_at"})}).Create(&row).Error; err != nil {
			return err
		}
		return writeCapabilityPlanAudit(tx, actorID, "ops.assistant_scenario_capabilities.default_set", "assistant_scenario_capabilities", "platform_default",
			map[string]any{"scope": "platform_shared", "external_search": capabilities.ExternalSearch, "mcp": capabilities.MCP, "tools": capabilities.Tools}, now)
	})
}

func (r *capabilityPlanRepository) ResolveScenario(ctx context.Context, tenantID uint64) (*types.ResolvedAssistantScenarioCapabilities, error) {
	if tenantID == 0 {
		return nil, ErrCapabilityTenantNotFound
	}
	var row struct {
		TenantPresent  uint64 `gorm:"column:tenant_present"`
		ExternalSearch bool   `gorm:"column:external_search"`
		MCP            bool   `gorm:"column:mcp"`
		Tools          bool   `gorm:"column:tools"`
		Source         string `gorm:"column:resolution_source"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT tenant.id AS tenant_present,
		       COALESCE(tenant_policy.external_search, platform_policy.external_search, ?) AS external_search,
		       COALESCE(tenant_policy.mcp, platform_policy.mcp, ?) AS mcp,
		       COALESCE(tenant_policy.tools, platform_policy.tools, ?) AS tools,
		       CASE WHEN tenant_policy.tenant_id IS NULL THEN 'platform_default' ELSE 'tenant_assignment' END AS resolution_source
		FROM tenants tenant
		LEFT JOIN tenant_assistant_scenario_capability_overrides tenant_policy ON tenant_policy.tenant_id = tenant.id
		LEFT JOIN assistant_scenario_capability_default platform_policy ON platform_policy.singleton_key = ?
		WHERE tenant.id = ? AND tenant.deleted_at IS NULL`, false, false, false, true, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.TenantPresent == 0 {
		return nil, nil
	}
	return &types.ResolvedAssistantScenarioCapabilities{Capabilities: scenarioCapabilities(row.ExternalSearch, row.MCP, row.Tools), Source: row.Source}, nil
}

func scenarioCapabilities(externalSearch, mcp, tools bool) types.AssistantScenarioCapabilities {
	return types.AssistantScenarioCapabilities{ExternalSearch: externalSearch, MCP: mcp, Tools: tools}
}

func (r *capabilityPlanRepository) SetTenantScenario(
	ctx context.Context, actorID string, tenantID uint64, capabilities types.AssistantScenarioCapabilities, now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorID); err != nil {
			return err
		}
		if err := requireCapabilityTenant(tx, tenantID); err != nil {
			return err
		}
		row := types.TenantAssistantScenarioCapabilityOverride{TenantID: tenantID, ExternalSearch: capabilities.ExternalSearch,
			MCP: capabilities.MCP, Tools: capabilities.Tools, UpdatedBy: actorID, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}}, DoUpdates: clause.AssignmentColumns(
			[]string{"external_search", "mcp", "tools", "updated_by", "updated_at"})}).Create(&row).Error; err != nil {
			return err
		}
		return writeCapabilityPlanAudit(tx, actorID, "ops.assistant_scenario_capabilities.tenant_set", "tenant", formatTenantID(tenantID),
			map[string]any{"scope": "enterprise_assigned", "external_search": capabilities.ExternalSearch, "mcp": capabilities.MCP, "tools": capabilities.Tools}, now)
	})
}

func (r *capabilityPlanRepository) ClearTenantScenario(
	ctx context.Context, actorID string, tenantID uint64, now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorID); err != nil {
			return err
		}
		if err := requireCapabilityTenant(tx, tenantID); err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&types.TenantAssistantScenarioCapabilityOverride{}).Error; err != nil {
			return err
		}
		return writeCapabilityPlanAudit(tx, actorID, "ops.assistant_scenario_capabilities.tenant_inherited", "tenant", formatTenantID(tenantID),
			map[string]any{"scope": "platform_shared"}, now)
	})
}

func (r *capabilityPlanRepository) GetTenant(ctx context.Context, tenantID uint64) (*types.Tenant, error) {
	if tenantID == 0 {
		return nil, ErrCapabilityTenantNotFound
	}
	var tenant types.Tenant
	if err := r.db.WithContext(ctx).Where("id = ?", tenantID).Take(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCapabilityTenantNotFound
		}
		return nil, err
	}
	return &tenant, nil
}
