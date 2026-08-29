package repository

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func activationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "activation.db") + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.User{}, &types.TenantMember{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	return db
}

func activationCommand(id, owner string) interfaces.EnterpriseActivationCommand {
	return interfaces.EnterpriseActivationCommand{
		ActivationID:      id,
		RequestSHA256:     strings.Repeat("a", 64),
		TenantName:        "Acme",
		TenantDescription: "Acme workspace",
		FirstOwnerUserID:  owner,
		DesiredState:      types.EnterpriseActivationStatePrepared,
	}
}

func createActivationUser(t *testing.T, db *gorm.DB, user types.User) {
	t.Helper()
	if user.Username == "" {
		user.Username = user.ID
	}
	if user.Email == "" {
		user.Email = user.ID + "@example.invalid"
	}
	query := db
	if user.TenantID == 0 {
		query = query.Omit("TenantID")
	}
	require.NoError(t, query.Create(&user).Error)
}

func TestEnterpriseActivationIdenticalReplayAndConcurrentUniqueness(t *testing.T) {
	db := activationTestDB(t)
	repo := NewTenantRepository(db)
	createActivationUser(t, db, types.User{ID: "owner-1", IsActive: true})
	command := activationCommand("activation-1", "owner-1")

	const calls = 8
	results := make([]*interfaces.EnterpriseActivationResult, calls)
	errs := make([]error, calls)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] = repo.ApplyEnterpriseActivation(context.Background(), command)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "call %d", i)
		require.NotNil(t, results[i])
		require.Equal(t, results[0].TenantID, results[i].TenantID)
		require.Equal(t, results[0].OwnerMembershipID, results[i].OwnerMembershipID)
		require.Equal(t, types.EnterpriseActivationStatePrepared, results[i].State)
	}
	var tenants, members int64
	require.NoError(t, db.Model(&types.Tenant{}).Count(&tenants).Error)
	require.NoError(t, db.Model(&types.TenantMember{}).Count(&members).Error)
	require.Equal(t, int64(1), tenants)
	require.Equal(t, int64(1), members)

	// The receipt remains reserved even after soft deletion.
	require.NoError(t, db.Delete(&types.Tenant{}, results[0].TenantID).Error)
	duplicateID := command.ActivationID
	duplicate := &types.Tenant{Name: "replacement", RingxunActivationID: &duplicateID}
	require.Error(t, db.Create(duplicate).Error)
}

func TestEnterpriseActivationConcurrentPostgres(t *testing.T) {
	newDatabase := func(t *testing.T) *gorm.DB {
		t.Helper()
		db := newIsolatedPostgresTestDatabase(t, "weknora_activation_")
		require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.User{}, &types.TenantMember{}))
		return db
	}
	runPair := func(repo interfaces.TenantRepository, commands [2]interfaces.EnterpriseActivationCommand) ([2]*interfaces.EnterpriseActivationResult, [2]error) {
		var results [2]*interfaces.EnterpriseActivationResult
		var errs [2]error
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range commands {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				<-start
				results[index], errs[index] = repo.ApplyEnterpriseActivation(context.Background(), commands[index])
			}(i)
		}
		close(start)
		wg.Wait()
		return results, errs
	}

	t.Run("identical replay", func(t *testing.T) {
		db := newDatabase(t)
		createActivationUser(t, db, types.User{ID: "owner-1", IsActive: true})
		command := activationCommand("activation-identical", "owner-1")
		results, errs := runPair(NewTenantRepository(db), [2]interfaces.EnterpriseActivationCommand{command, command})

		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		require.Equal(t, results[0].TenantID, results[1].TenantID)
		require.Equal(t, results[0].OwnerMembershipID, results[1].OwnerMembershipID)
		var tenants, members int64
		require.NoError(t, db.Model(&types.Tenant{}).Where("ringxun_activation_id = ?", command.ActivationID).Count(&tenants).Error)
		require.NoError(t, db.Model(&types.TenantMember{}).Count(&members).Error)
		require.Equal(t, int64(1), tenants)
		require.Equal(t, int64(1), members)
	})

	t.Run("different owners", func(t *testing.T) {
		db := newDatabase(t)
		createActivationUser(t, db, types.User{ID: "owner-1", IsActive: true})
		createActivationUser(t, db, types.User{ID: "owner-2", IsActive: true})
		first := activationCommand("activation-owner-race", "owner-1")
		second := activationCommand("activation-owner-race", "owner-2")
		_, errs := runPair(NewTenantRepository(db), [2]interfaces.EnterpriseActivationCommand{first, second})

		var succeeded, conflicted int
		for _, err := range errs {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, ErrEnterpriseActivationConflict):
				conflicted++
			default:
				t.Fatalf("unexpected activation result: %v", err)
			}
		}
		require.Equal(t, 1, succeeded)
		require.Equal(t, 1, conflicted)
		var tenant types.Tenant
		require.NoError(t, db.Where("ringxun_activation_id = ?", first.ActivationID).Take(&tenant).Error)
		var tenants, members, boundUsers int64
		require.NoError(t, db.Model(&types.Tenant{}).Where("ringxun_activation_id = ?", first.ActivationID).Count(&tenants).Error)
		require.NoError(t, db.Model(&types.TenantMember{}).Count(&members).Error)
		require.NoError(t, db.Model(&types.User{}).Where("tenant_id = ?", tenant.ID).Count(&boundUsers).Error)
		require.Equal(t, int64(1), tenants)
		require.Equal(t, int64(1), members)
		require.Equal(t, int64(1), boundUsers)
	})
}

func TestEnterpriseActivationAcceptsSQLNullTenantlessUser(t *testing.T) {
	db := activationTestDB(t)
	// PostgreSQL stores pre-enterprise tenantless identities as NULL. SQLite
	// uses the same three-valued predicate semantics for this equivalent test.
	createActivationUser(t, db, types.User{ID: "null-owner", IsActive: true})

	result, err := NewTenantRepository(db).ApplyEnterpriseActivation(
		context.Background(), activationCommand("activation-null-owner", "null-owner"))
	require.NoError(t, err)
	require.NotZero(t, result.TenantID)

	var user types.User
	require.NoError(t, db.First(&user, "id = ?", "null-owner").Error)
	require.Equal(t, result.TenantID, user.TenantID)
}

func TestEnterpriseActivationReceiptConflictsDoNotMutate(t *testing.T) {
	db := activationTestDB(t)
	repo := NewTenantRepository(db)
	createActivationUser(t, db, types.User{ID: "owner-1", IsActive: true})
	createActivationUser(t, db, types.User{ID: "owner-2", IsActive: true})
	base := activationCommand("activation-conflict", "owner-1")
	created, err := repo.ApplyEnterpriseActivation(context.Background(), base)
	require.NoError(t, err)

	tests := []struct {
		name   string
		mutate func(*interfaces.EnterpriseActivationCommand)
	}{
		{name: "digest", mutate: func(c *interfaces.EnterpriseActivationCommand) { c.RequestSHA256 = strings.Repeat("b", 64) }},
		{name: "owner", mutate: func(c *interfaces.EnterpriseActivationCommand) { c.FirstOwnerUserID = "owner-2" }},
		{name: "tenant name", mutate: func(c *interfaces.EnterpriseActivationCommand) { c.TenantName = "Other" }},
		{name: "tenant description", mutate: func(c *interfaces.EnterpriseActivationCommand) { c.TenantDescription = "Other" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := base
			tt.mutate(&command)
			_, err := repo.ApplyEnterpriseActivation(context.Background(), command)
			require.ErrorIs(t, err, ErrEnterpriseActivationConflict)

			var tenant types.Tenant
			require.NoError(t, db.First(&tenant, created.TenantID).Error)
			require.Equal(t, types.TenantStatusProvisioning, tenant.Status)
			require.Equal(t, base.TenantName, tenant.Name)
			require.Equal(t, base.TenantDescription, tenant.Description)
			var owner2 types.User
			require.NoError(t, db.First(&owner2, "id = ?", "owner-2").Error)
			require.Zero(t, owner2.TenantID)
			var count int64
			require.NoError(t, db.Model(&types.TenantMember{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
		})
	}
}

func TestEnterpriseActivationRejectsInvalidInitialOwnerWithoutMutation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *gorm.DB)
	}{
		{name: "missing"},
		{name: "inactive", setup: func(t *testing.T, db *gorm.DB) {
			createActivationUser(t, db, types.User{ID: "candidate", IsActive: true})
			require.NoError(t, db.Model(&types.User{}).Where("id = ?", "candidate").Update("is_active", false).Error)
		}},
		{name: "system", setup: func(t *testing.T, db *gorm.DB) {
			createActivationUser(t, db, types.User{ID: "candidate", IsActive: true, IsSystemAdmin: true})
		}},
		{name: "foreign bound", setup: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Create(&types.Tenant{ID: 77, Name: "foreign", Status: types.TenantStatusActive}).Error)
			createActivationUser(t, db, types.User{ID: "candidate", IsActive: true, TenantID: 77})
		}},
		{name: "existing non-owner membership", setup: func(t *testing.T, db *gorm.DB) {
			createActivationUser(t, db, types.User{ID: "candidate", IsActive: true})
			tenant := &types.Tenant{Name: "existing", Status: types.TenantStatusActive}
			require.NoError(t, db.Create(tenant).Error)
			require.NoError(t, db.Create(&types.TenantMember{
				UserID: "candidate", TenantID: tenant.ID,
				Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive,
			}).Error)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := activationTestDB(t)
			if tt.setup != nil {
				tt.setup(t, db)
			}
			repo := NewTenantRepository(db)
			_, err := repo.ApplyEnterpriseActivation(context.Background(), activationCommand("activation-invalid", "candidate"))
			require.ErrorIs(t, err, ErrEnterpriseActivationConflict)

			var receipts int64
			require.NoError(t, db.Unscoped().Model(&types.Tenant{}).
				Where("ringxun_activation_id IS NOT NULL").Count(&receipts).Error)
			require.Zero(t, receipts)
			var user types.User
			if err := db.First(&user, "id = ?", "candidate").Error; !errors.Is(err, gorm.ErrRecordNotFound) {
				require.NoError(t, err)
				if tt.name != "foreign bound" {
					require.Zero(t, user.TenantID)
				}
			}
		})
	}
}

func TestEnterpriseActivationStateTransitionsAreAtomicAndTerminal(t *testing.T) {
	db := activationTestDB(t)
	repo := NewTenantRepository(db)
	createActivationUser(t, db, types.User{ID: "active-owner", IsActive: true})
	activeCommand := activationCommand("activation-active", "active-owner")
	prepared, err := repo.ApplyEnterpriseActivation(context.Background(), activeCommand)
	require.NoError(t, err)
	require.NoError(t, db.Model(&types.Tenant{}).Where("id = ?", prepared.TenantID).
		Update("default_storage_backend_id", "backend-1").Error)
	activeCommand.DesiredState = types.EnterpriseActivationStateActive
	active, err := repo.ApplyEnterpriseActivation(context.Background(), activeCommand)
	require.NoError(t, err)
	require.Equal(t, prepared.TenantID, active.TenantID)
	require.Equal(t, prepared.OwnerMembershipID, active.OwnerMembershipID)
	require.Equal(t, types.EnterpriseActivationStateActive, active.State)

	var activeTenant types.Tenant
	require.NoError(t, db.First(&activeTenant, active.TenantID).Error)
	var activeMember types.TenantMember
	require.NoError(t, db.First(&activeMember, active.OwnerMembershipID).Error)
	require.Equal(t, types.TenantStatusActive, activeTenant.Status)
	require.Equal(t, types.TenantMemberStatusActive, activeMember.Status)
	activeCommand.DesiredState = types.EnterpriseActivationStateAbandoned
	_, err = repo.ApplyEnterpriseActivation(context.Background(), activeCommand)
	require.ErrorIs(t, err, ErrEnterpriseActivationConflict)

	createActivationUser(t, db, types.User{ID: "abandoned-owner", IsActive: true})
	abandonedCommand := activationCommand("activation-abandoned", "abandoned-owner")
	_, err = repo.ApplyEnterpriseActivation(context.Background(), abandonedCommand)
	require.NoError(t, err)
	abandonedCommand.DesiredState = types.EnterpriseActivationStateAbandoned
	abandoned, err := repo.ApplyEnterpriseActivation(context.Background(), abandonedCommand)
	require.NoError(t, err)
	require.Equal(t, types.EnterpriseActivationStateAbandoned, abandoned.State)
	abandonedCommand.DesiredState = types.EnterpriseActivationStateActive
	_, err = repo.ApplyEnterpriseActivation(context.Background(), abandonedCommand)
	require.ErrorIs(t, err, ErrEnterpriseActivationConflict)

	var abandonedTenant types.Tenant
	require.NoError(t, db.First(&abandonedTenant, abandoned.TenantID).Error)
	var abandonedMember types.TenantMember
	require.NoError(t, db.First(&abandonedMember, abandoned.OwnerMembershipID).Error)
	require.Equal(t, types.TenantStatusActivationAbandoned, abandonedTenant.Status)
	require.Equal(t, types.TenantMemberStatusSuspended, abandonedMember.Status)
}
