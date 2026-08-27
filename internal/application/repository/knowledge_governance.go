package repository

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrBusinessRoleNotFound          = errors.New("business role not found")
	ErrKnowledgeAccessTargetNotFound = errors.New("knowledge access target not found")
	ErrKnowledgeAccessRoleInvalid    = errors.New("knowledge access role is invalid")
)

type knowledgeGovernanceService struct {
	db    *gorm.DB
	audit interfaces.AuditLogService
}

func NewKnowledgeGovernanceService(db *gorm.DB, audit interfaces.AuditLogService) interfaces.KnowledgeGovernanceService {
	return &knowledgeGovernanceService{db: db, audit: audit}
}

func (s *knowledgeGovernanceService) emitAudit(ctx context.Context, tenantID uint64, action types.AuditAction, targetType, targetID, targetUserID string, details map[string]interface{}) {
	if s.audit == nil {
		return
	}
	actorID, _ := types.UserIDFromContext(ctx)
	payload, _ := json.Marshal(details)
	_ = s.audit.Log(ctx, &types.AuditLog{TenantID: tenantID, ActorUserID: actorID, ActorRole: string(types.TenantRoleFromContext(ctx)), Action: action, TargetType: targetType, TargetID: targetID, TargetUserID: targetUserID, Outcome: types.AuditOutcomeSuccess, Details: types.JSON(payload)})
}

func (s *knowledgeGovernanceService) ListBusinessRoles(ctx context.Context, tenantID uint64) ([]*types.BusinessRole, error) {
	var roles []*types.BusinessRole
	err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("name ASC, id ASC").Find(&roles).Error
	return roles, err
}

func (s *knowledgeGovernanceService) CreateBusinessRole(ctx context.Context, tenantID uint64, name string) (*types.BusinessRole, error) {
	role := &types.BusinessRole{ID: uuid.NewString(), TenantID: tenantID, Name: strings.TrimSpace(name), Enabled: true}
	if err := s.db.WithContext(ctx).Create(role).Error; err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantID, types.AuditActionKnowledgeRoleCreated, "business_role", role.ID, "", map[string]interface{}{"name": role.Name})
	return role, nil
}

func (s *knowledgeGovernanceService) UpdateBusinessRole(ctx context.Context, tenantID uint64, id, name string, enabled bool) (*types.BusinessRole, error) {
	var role types.BusinessRole
	if err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBusinessRoleNotFound
		}
		return nil, err
	}
	role.Name, role.Enabled = strings.TrimSpace(name), enabled
	if err := s.db.WithContext(ctx).Save(&role).Error; err != nil {
		return nil, err
	}
	s.emitAudit(ctx, tenantID, types.AuditActionKnowledgeRoleUpdated, "business_role", role.ID, "", map[string]interface{}{"name": role.Name, "enabled": role.Enabled})
	return &role, nil
}

func distinctSorted(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			seen[id] = struct{}{}
		}
	}
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *knowledgeGovernanceService) activeRolesLocked(ctx context.Context, tx *gorm.DB, tenantID uint64, ids []string) ([]types.BusinessRole, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Stable order prevents mutually inverted row locks. The second statement
	// after FOR UPDATE is intentional: PostgreSQL READ COMMITTED must not use
	// a pre-wait status snapshot for a decision about a newly disabled role.
	var locked []types.BusinessRole
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Where("tenant_id = ? AND id IN ?", tenantID, ids).Order("id ASC").Find(&locked).Error; err != nil {
		return nil, err
	}
	var current []types.BusinessRole
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND enabled = ? AND id IN ?", tenantID, true, ids).Order("id ASC").Find(&current).Error; err != nil {
		return nil, err
	}
	if len(current) != len(ids) {
		return nil, ErrKnowledgeAccessRoleInvalid
	}
	return current, nil
}

func (s *knowledgeGovernanceService) ReplaceMemberBusinessRoles(ctx context.Context, tenantID uint64, userID string, roleIDs []string) error {
	roleIDs = distinctSorted(roleIDs)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var member types.TenantMember
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Where("tenant_id = ? AND user_id = ?", tenantID, userID).First(&member).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrKnowledgeAccessTargetNotFound
			}
			return err
		}
		var currentIDs []string
		if err := tx.WithContext(ctx).Model(&types.BusinessRoleMember{}).Where("tenant_id = ? AND tenant_member_id = ?", tenantID, member.ID).Pluck("role_id", &currentIDs).Error; err != nil {
			return err
		}
		if _, err := s.activeRolesLocked(ctx, tx, tenantID, newlyAssignedRoleIDs(currentIDs, roleIDs)); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("tenant_id = ? AND tenant_member_id = ?", tenantID, member.ID).Delete(&types.BusinessRoleMember{}).Error; err != nil {
			return err
		}
		rows := make([]types.BusinessRoleMember, 0, len(roleIDs))
		for _, id := range roleIDs {
			rows = append(rows, types.BusinessRoleMember{TenantID: tenantID, RoleID: id, TenantMemberID: member.ID})
		}
		if len(rows) > 0 {
			return tx.WithContext(ctx).Create(&rows).Error
		}
		return nil
	})
	if err == nil {
		s.emitAudit(ctx, tenantID, types.AuditActionKnowledgeRoleMemberAssigned, "tenant_member", userID, userID, map[string]interface{}{"role_ids": roleIDs})
	}
	return err
}

func (s *knowledgeGovernanceService) ListMemberBusinessRoleIDs(ctx context.Context, tenantID uint64, userID string) ([]string, error) {
	var ids []string
	err := s.db.WithContext(ctx).Model(&types.BusinessRoleMember{}).Select("business_role_members.role_id").
		Joins("INNER JOIN business_roles ON business_roles.id = business_role_members.role_id AND business_roles.tenant_id = business_role_members.tenant_id AND business_roles.deleted_at IS NULL").
		Joins("INNER JOIN tenant_members ON tenant_members.id = business_role_members.tenant_member_id AND tenant_members.tenant_id = business_role_members.tenant_id AND tenant_members.deleted_at IS NULL").
		Where("business_role_members.tenant_id = ? AND tenant_members.user_id = ?", tenantID, userID).Order("business_role_members.role_id ASC").Scan(&ids).Error
	return ids, err
}

func (s *knowledgeGovernanceService) GetKnowledgeBaseRoleGrants(ctx context.Context, tenantID uint64, kbID string) ([]string, error) {
	var ids []string
	err := s.db.WithContext(ctx).Model(&types.KnowledgeBaseRoleGrant{}).Select("role_id").Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).Order("role_id ASC").Scan(&ids).Error
	return ids, err
}

func (s *knowledgeGovernanceService) ReplaceKnowledgeBaseRoleGrants(ctx context.Context, tenantID uint64, kbID string, mode string, roleIDs []string) error {
	roleIDs = distinctSorted(roleIDs)
	if (mode == "all" && len(roleIDs) != 0) || (mode == "roles" && len(roleIDs) == 0) || (mode != "all" && mode != "roles") {
		return ErrKnowledgeAccessRoleInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var kb types.KnowledgeBase
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id").Where("id = ? AND tenant_id = ?", kbID, tenantID).Take(&kb).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrKnowledgeAccessTargetNotFound
			}
			return err
		}
		var currentIDs []string
		if err := tx.WithContext(ctx).Model(&types.KnowledgeBaseRoleGrant{}).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).Pluck("role_id", &currentIDs).Error; err != nil {
			return err
		}
		if _, err := s.activeRolesLocked(ctx, tx, tenantID, newlyAssignedRoleIDs(currentIDs, roleIDs)); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).Delete(&types.KnowledgeBaseRoleGrant{}).Error; err != nil {
			return err
		}
		rows := make([]types.KnowledgeBaseRoleGrant, 0, len(roleIDs))
		for _, id := range roleIDs {
			rows = append(rows, types.KnowledgeBaseRoleGrant{TenantID: tenantID, KnowledgeBaseID: kbID, RoleID: id})
		}
		if len(rows) > 0 {
			return tx.WithContext(ctx).Create(&rows).Error
		}
		return nil
	})
	if err == nil {
		s.emitAudit(ctx, tenantID, types.AuditActionKnowledgeAccessUpdated, "knowledge_base", kbID, "", map[string]interface{}{"mode": mode, "role_ids": roleIDs})
	}
	return err
}

// newlyAssignedRoleIDs keeps disabled relationships removable: replacements only
// validate IDs that are entering the set, never rows they merely retain or remove.
func newlyAssignedRoleIDs(currentIDs, requestedIDs []string) []string {
	current := make(map[string]struct{}, len(currentIDs))
	for _, id := range currentIDs {
		current[id] = struct{}{}
	}
	newIDs := make([]string, 0, len(requestedIDs))
	for _, id := range requestedIDs {
		if _, exists := current[id]; !exists {
			newIDs = append(newIDs, id)
		}
	}
	return newIDs
}

func (s *knowledgeGovernanceService) CanAccessKnowledgeBase(ctx context.Context, tenantID uint64, userID string, tenantRole types.TenantRole, kbID string) (bool, error) {
	if _, apiKey := types.TenantAPIKeyScopeFromContext(ctx); apiKey || userID == "" || types.IsSyntheticUserID(userID) {
		return true, nil
	}
	var member types.TenantMember
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND user_id = ?", tenantID, userID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if member.Status != types.TenantMemberStatusActive {
		return false, nil
	}
	if member.Role.HasPermission(types.TenantRoleContributor) {
		return true, nil
	}
	var grants int64
	if err := s.db.WithContext(ctx).Model(&types.KnowledgeBaseRoleGrant{}).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).Count(&grants).Error; err != nil {
		return false, err
	}
	if grants == 0 {
		return true, nil
	}
	var matched int64
	err := s.db.WithContext(ctx).Model(&types.KnowledgeBaseRoleGrant{}).
		Joins("INNER JOIN business_roles ON business_roles.id = knowledge_base_role_grants.role_id AND business_roles.tenant_id = knowledge_base_role_grants.tenant_id AND business_roles.enabled = ? AND business_roles.deleted_at IS NULL", true).
		Joins("INNER JOIN business_role_members ON business_role_members.role_id = knowledge_base_role_grants.role_id AND business_role_members.tenant_id = knowledge_base_role_grants.tenant_id AND business_role_members.tenant_member_id = ?", member.ID).
		Where("knowledge_base_role_grants.tenant_id = ? AND knowledge_base_role_grants.knowledge_base_id = ?", tenantID, kbID).Count(&matched).Error
	return matched > 0, err
}

func (s *knowledgeGovernanceService) FilterKnowledgeBases(ctx context.Context, tenantID uint64, userID string, tenantRole types.TenantRole, kbs []*types.KnowledgeBase) ([]*types.KnowledgeBase, error) {
	if userID == "" || types.IsSyntheticUserID(userID) {
		return kbs, nil
	}
	out := make([]*types.KnowledgeBase, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil || kb.TenantID != tenantID {
			continue
		}
		ok, err := s.CanAccessKnowledgeBase(ctx, tenantID, userID, tenantRole, kb.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, kb)
		}
	}
	return out, nil
}
