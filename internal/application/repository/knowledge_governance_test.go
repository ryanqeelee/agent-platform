package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newKnowledgeGovernanceTestService(t *testing.T) interfaces.KnowledgeGovernanceService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.KnowledgeBase{}, &types.TenantMember{}, &types.BusinessRole{}, &types.BusinessRoleMember{}, &types.KnowledgeBaseRoleGrant{}))
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb", TenantID: 1}).Error)
	require.NoError(t, db.Create(&types.TenantMember{TenantID: 1, UserID: "employee", Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	require.NoError(t, db.Create(&types.TenantMember{TenantID: 1, UserID: "admin", Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive}).Error)
	return NewKnowledgeGovernanceService(db, nil)
}

func TestKnowledgeGovernanceAccessTruthTable(t *testing.T) {
	ctx := context.Background()
	service := newKnowledgeGovernanceTestService(t)
	// Empty grants are enterprise-wide; higher tenant roles bypass role grants.
	allowed, err := service.CanAccessKnowledgeBase(ctx, 1, "employee", types.TenantRoleViewer, "kb")
	require.NoError(t, err)
	require.True(t, allowed)
	role, err := service.CreateBusinessRole(ctx, 1, "Store manager")
	require.NoError(t, err)
	require.NoError(t, service.ReplaceKnowledgeBaseRoleGrants(ctx, 1, "kb", "roles", []string{role.ID}))
	allowed, err = service.CanAccessKnowledgeBase(ctx, 1, "employee", types.TenantRoleViewer, "kb")
	require.NoError(t, err)
	require.False(t, allowed, "unmatched employee must fail closed")
	require.NoError(t, service.ReplaceMemberBusinessRoles(ctx, 1, "employee", []string{role.ID}))
	allowed, err = service.CanAccessKnowledgeBase(ctx, 1, "employee", types.TenantRoleViewer, "kb")
	require.NoError(t, err)
	require.True(t, allowed)
	_, err = service.UpdateBusinessRole(ctx, 1, role.ID, role.Name, false)
	require.NoError(t, err)
	allowed, err = service.CanAccessKnowledgeBase(ctx, 1, "employee", types.TenantRoleViewer, "kb")
	require.NoError(t, err)
	require.False(t, allowed, "disabled role pauses access without deleting assignments")
	allowed, err = service.CanAccessKnowledgeBase(ctx, 1, "admin", types.TenantRoleAdmin, "kb")
	require.NoError(t, err)
	require.True(t, allowed)
	// A disabled relationship can be retained or removed, but cannot be newly
	// added to a member or KB scope.
	require.NoError(t, service.ReplaceMemberBusinessRoles(ctx, 1, "employee", []string{role.ID}))
	require.NoError(t, service.ReplaceKnowledgeBaseRoleGrants(ctx, 1, "kb", "roles", []string{role.ID}))
	other, err := service.CreateBusinessRole(ctx, 1, "Other")
	require.NoError(t, err)
	_, err = service.UpdateBusinessRole(ctx, 1, other.ID, other.Name, false)
	require.NoError(t, err)
	err = service.ReplaceKnowledgeBaseRoleGrants(ctx, 1, "kb", "roles", []string{other.ID})
	require.True(t, errors.Is(err, ErrKnowledgeAccessRoleInvalid))
	grants, err := service.GetKnowledgeBaseRoleGrants(ctx, 1, "kb")
	require.NoError(t, err)
	require.Equal(t, []string{role.ID}, grants)
}

func TestKnowledgeGovernanceUsesCurrentMembershipNotStaleTokenRole(t *testing.T) {
	ctx := context.Background()
	service := newKnowledgeGovernanceTestService(t)
	role, err := service.CreateBusinessRole(ctx, 1, "cashier")
	require.NoError(t, err)
	require.NoError(t, service.ReplaceKnowledgeBaseRoleGrants(ctx, 1, "kb", "roles", []string{role.ID}))

	// A stale Contributor token cannot bypass a currently suspended member.
	db := service.(*knowledgeGovernanceService).db
	require.NoError(t, db.Model(&types.TenantMember{}).Where("tenant_id = ? AND user_id = ?", 1, "employee").Update("status", types.TenantMemberStatusSuspended).Error)
	allowed, err := service.CanAccessKnowledgeBase(ctx, 1, "employee", types.TenantRoleContributor, "kb")
	require.NoError(t, err)
	require.False(t, allowed)

	// Once active but demoted, the old Admin token also cannot bypass grants.
	require.NoError(t, db.Model(&types.TenantMember{}).Where("tenant_id = ? AND user_id = ?", 1, "employee").Updates(map[string]interface{}{"status": types.TenantMemberStatusActive, "role": types.TenantRoleViewer}).Error)
	allowed, err = service.CanAccessKnowledgeBase(ctx, 1, "employee", types.TenantRoleAdmin, "kb")
	require.NoError(t, err)
	require.False(t, allowed)
	require.NoError(t, service.ReplaceMemberBusinessRoles(ctx, 1, "employee", []string{role.ID}))
	allowed, err = service.CanAccessKnowledgeBase(ctx, 1, "employee", types.TenantRoleAdmin, "kb")
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestKnowledgeGovernanceRejoinedMemberDoesNotInheritRoles(t *testing.T) {
	ctx := context.Background()
	service := newKnowledgeGovernanceTestService(t)
	role, err := service.CreateBusinessRole(ctx, 1, "cashier")
	require.NoError(t, err)
	require.NoError(t, service.ReplaceMemberBusinessRoles(ctx, 1, "employee", []string{role.ID}))
	db := service.(*knowledgeGovernanceService).db
	require.NoError(t, db.Where("tenant_id = ? AND user_id = ?", 1, "employee").Delete(&types.TenantMember{}).Error)
	require.NoError(t, db.Create(&types.TenantMember{TenantID: 1, UserID: "employee", Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	ids, err := service.ListMemberBusinessRoleIDs(ctx, 1, "employee")
	require.NoError(t, err)
	require.Empty(t, ids)
}
