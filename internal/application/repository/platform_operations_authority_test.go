package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestPlatformOperationsRechecksSystemAdministratorInsideEmployeeTransaction(t *testing.T) {
	db := activationTestDB(t)
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
	require.NoError(t, db.Create(tenant).Error)
	require.NoError(t, db.Omit("TenantID").Create(&types.User{
		ID: "system-admin", Username: "system-admin", Email: "system-admin@example.invalid",
		IsActive: true, IsSystemAdmin: true,
	}).Error)
	repo := &tenantMemberRepository{db: db}
	create := func(id string) error {
		user := &types.User{ID: id, Username: id, Email: id + "@example.invalid", TenantID: tenant.ID, IsActive: true}
		member := &types.TenantMember{UserID: id, TenantID: tenant.ID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}
		return repo.CreateEmployee(context.Background(), types.MemberActorAuthority{
			UserID: "system-admin", SystemAdministrator: true,
		}, user, member)
	}
	require.NoError(t, create("employee-1"))
	require.NoError(t, db.Model(&types.User{}).Where("id = ?", "system-admin").Update("is_system_admin", false).Error)
	require.ErrorIs(t, create("employee-2"), ErrMemberActionForbidden)
	var count int64
	require.NoError(t, db.Model(&types.User{}).Where("id = ?", "employee-2").Count(&count).Error)
	require.Zero(t, count)
}

func TestPlatformOperationsRejectsEmployeeCreationForInactiveEnterprise(t *testing.T) {
	db := activationTestDB(t)
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusProvisioning}
	require.NoError(t, db.Create(tenant).Error)
	require.NoError(t, db.Omit("TenantID").Create(&types.User{
		ID: "system-admin", Username: "system-admin", Email: "system-admin@example.invalid",
		IsActive: true, IsSystemAdmin: true,
	}).Error)
	repo := &tenantMemberRepository{db: db}
	user := &types.User{ID: "employee", Username: "employee", Email: "employee@example.invalid", TenantID: tenant.ID, IsActive: true}
	member := &types.TenantMember{UserID: user.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}
	err := repo.CreateEmployee(context.Background(), types.MemberActorAuthority{
		UserID: "system-admin", SystemAdministrator: true,
	}, user, member)
	require.True(t, errors.Is(err, ErrEnterpriseNotActive), "err=%v", err)
	one := 1
	_, _, err = NewPlatformOperationsTenantRepository(db).UpdateForPlatformOperations(
		context.Background(), "system-admin", tenant.ID, tenant.Name, tenant.Description,
		types.TenantStatusActive, false, &one, 0)
	require.ErrorIs(t, err, ErrEnterpriseStatusImmutable)
	var unchanged types.Tenant
	require.NoError(t, db.First(&unchanged, tenant.ID).Error)
	require.Equal(t, types.TenantStatusProvisioning, unchanged.Status)
}

func TestPlatformOperationsSeatReductionCountsOnlyActiveBoundMembers(t *testing.T) {
	db := activationTestDB(t)
	three := 3
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive, SeatsTotal: &three}
	other := &types.Tenant{Name: "Other", Status: types.TenantStatusActive}
	require.NoError(t, db.Create(tenant).Error)
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Omit("TenantID").Create(&types.User{
		ID: "system-admin", Username: "system-admin", Email: "system-admin@example.invalid",
		IsActive: true, IsSystemAdmin: true,
	}).Error)
	for _, user := range []types.User{
		{ID: "bound", Username: "bound", Email: "bound@example.invalid", TenantID: tenant.ID, IsActive: true},
		{ID: "stale", Username: "stale", Email: "stale@example.invalid", TenantID: other.ID, IsActive: true},
	} {
		require.NoError(t, db.Create(&user).Error)
	}
	for _, userID := range []string{"bound", "stale"} {
		require.NoError(t, db.Create(&types.TenantMember{
			UserID: userID, TenantID: tenant.ID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive,
		}).Error)
	}
	one := 1
	updated, used, err := NewPlatformOperationsTenantRepository(db).UpdateForPlatformOperations(
		context.Background(), "system-admin", tenant.ID, tenant.Name, tenant.Description,
		types.TenantStatusSuspended, true, &one, 8192)
	require.NoError(t, err)
	require.Equal(t, int64(1), used)
	require.Equal(t, 1, *updated.SeatsTotal)
	require.Equal(t, types.TenantStatusSuspended, updated.Status)
	require.Equal(t, int64(8192), updated.StorageQuota)
	require.NoError(t, db.Create(&types.User{
		ID: "bound-2", Username: "bound-2", Email: "bound-2@example.invalid", TenantID: tenant.ID, IsActive: true,
	}).Error)
	require.NoError(t, db.Create(&types.TenantMember{
		UserID: "bound-2", TenantID: tenant.ID, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive,
	}).Error)
	_, used, err = NewPlatformOperationsTenantRepository(db).UpdateForPlatformOperations(
		context.Background(), "system-admin", tenant.ID, tenant.Name, tenant.Description,
		types.TenantStatusActive, true, &one, 8192)
	require.ErrorIs(t, err, ErrSeatLimitBelowUsage)
	require.Equal(t, int64(2), used)
}

func TestPlatformMemberMutationsRecheckTargetUserInsideTransaction(t *testing.T) {
	mutations := map[string]func(*tenantMemberRepository, types.MemberActorAuthority, string, uint64) error{
		"role": func(repo *tenantMemberRepository, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
			return repo.UpdateRole(context.Background(), actor, userID, tenantID, types.TenantRoleAdmin)
		},
		"status": func(repo *tenantMemberRepository, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
			return repo.UpdateStatus(context.Background(), actor, userID, tenantID, types.TenantMemberStatusSuspended)
		},
		"remove": func(repo *tenantMemberRepository, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
			return repo.SoftDelete(context.Background(), actor, userID, tenantID)
		},
		"operating access": func(repo *tenantMemberRepository, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
			_, err := repo.UpdateOperatingAnalysisAccess(context.Background(), actor, userID, tenantID, true)
			return err
		},
	}
	targetStates := map[string]func(*testing.T, *tenantMemberRepository, *types.User){
		"inactive": func(t *testing.T, repo *tenantMemberRepository, user *types.User) {
			require.NoError(t, repo.db.Model(user).Update("is_active", false).Error)
		},
		"promoted system admin": func(t *testing.T, repo *tenantMemberRepository, user *types.User) {
			require.NoError(t, repo.db.Model(user).Update("is_system_admin", true).Error)
		},
		"cross tenant authority": func(t *testing.T, repo *tenantMemberRepository, user *types.User) {
			require.NoError(t, repo.db.Model(user).Update("can_access_all_tenants", true).Error)
		},
	}

	for mutationName, mutate := range mutations {
		for stateName, changeState := range targetStates {
			t.Run(mutationName+"/"+stateName, func(t *testing.T) {
				db := activationTestDB(t)
				tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
				require.NoError(t, db.Create(tenant).Error)
				require.NoError(t, db.Omit("TenantID").Create(&types.User{
					ID: "system-admin", Username: "system-admin", Email: "system-admin@example.invalid",
					IsActive: true, IsSystemAdmin: true,
				}).Error)
				target := &types.User{
					ID: "target", Username: "target", Email: "target@example.invalid",
					TenantID: tenant.ID, IsActive: true,
				}
				require.NoError(t, db.Create(target).Error)
				require.NoError(t, db.Create(&types.TenantMember{
					UserID: target.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer,
					Status: types.TenantMemberStatusActive,
				}).Error)
				repo := &tenantMemberRepository{db: db}
				preflight, err := repo.Get(context.Background(), target.ID, tenant.ID)
				require.NoError(t, err)
				require.NotNil(t, preflight)
				changeState(t, repo, target)

				err = mutate(repo, types.MemberActorAuthority{
					UserID: "system-admin", SystemAdministrator: true,
				}, target.ID, tenant.ID)
				require.ErrorIs(t, err, ErrMemberActionForbidden)
				var membership types.TenantMember
				require.NoError(t, db.Where("user_id = ? AND tenant_id = ?", target.ID, tenant.ID).Take(&membership).Error)
				require.Equal(t, types.TenantRoleViewer, membership.Role)
				require.Equal(t, types.TenantMemberStatusActive, membership.Status)
				require.False(t, membership.OperatingAnalysisAccess)
			})
		}
	}
}

func TestPlatformMemberMutationsRequireActiveEnterprise(t *testing.T) {
	mutations := map[string]func(*tenantMemberRepository, types.MemberActorAuthority, string, uint64) error{
		"role": func(repo *tenantMemberRepository, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
			return repo.UpdateRole(context.Background(), actor, userID, tenantID, types.TenantRoleAdmin)
		},
		"status": func(repo *tenantMemberRepository, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
			return repo.UpdateStatus(context.Background(), actor, userID, tenantID, types.TenantMemberStatusSuspended)
		},
		"remove": func(repo *tenantMemberRepository, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
			return repo.SoftDelete(context.Background(), actor, userID, tenantID)
		},
	}
	for _, status := range []string{types.TenantStatusSuspended, types.TenantStatusProvisioning, types.TenantStatusActivationAbandoned} {
		for name, mutate := range mutations {
			t.Run(status+"/"+name, func(t *testing.T) {
				db := activationTestDB(t)
				tenant := &types.Tenant{Name: "Acme", Status: status}
				require.NoError(t, db.Create(tenant).Error)
				require.NoError(t, db.Omit("TenantID").Create(&types.User{
					ID: "operator", Username: "operator", Email: "operator@example.invalid", IsActive: true, IsSystemAdmin: true,
				}).Error)
				target := &types.User{ID: "target", Username: "target", Email: "target@example.invalid", PasswordHash: "unused", TenantID: tenant.ID, IsActive: true}
				require.NoError(t, db.Create(target).Error)
				require.NoError(t, db.Create(&types.TenantMember{UserID: target.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
				err := mutate(&tenantMemberRepository{db: db}, types.MemberActorAuthority{UserID: "operator", SystemAdministrator: true}, target.ID, tenant.ID)
				require.ErrorIs(t, err, ErrEnterpriseNotActive)
				var member types.TenantMember
				require.NoError(t, db.Where("user_id = ? AND tenant_id = ?", target.ID, tenant.ID).Take(&member).Error)
				require.Equal(t, types.TenantRoleViewer, member.Role)
				require.Equal(t, types.TenantMemberStatusActive, member.Status)
			})
		}
	}
}
