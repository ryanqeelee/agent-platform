package repository

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newKnowledgeGovernancePostgresDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	schema := "weknora_knowledge_governance_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })

	scopedDSN := dsn + " search_path=" + schema
	if strings.Contains(dsn, "://") {
		parsed, parseErr := url.Parse(dsn)
		require.NoError(t, parseErr)
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		scopedDSN = parsed.String()
	}
	db, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{})
	require.NoError(t, err)
	for _, statement := range []string{
		`CREATE TABLE tenants (id bigint PRIMARY KEY)`,
		`CREATE TABLE knowledge_bases (id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, deleted_at timestamptz)`,
		`CREATE TABLE tenant_members (
			id bigserial PRIMARY KEY, user_id varchar(36) NOT NULL, tenant_id bigint NOT NULL,
			role varchar(20) NOT NULL, status varchar(20) NOT NULL, invited_by varchar(36), joined_at timestamptz,
			created_at timestamptz, updated_at timestamptz, deleted_at timestamptz
		)`,
		`CREATE UNIQUE INDEX tenant_members_active_unique ON tenant_members(tenant_id, user_id) WHERE deleted_at IS NULL`,
		`INSERT INTO tenants(id) VALUES (1)`,
		`INSERT INTO knowledge_bases(id, tenant_id) VALUES ('kb', 1)`,
		`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('employee', 1, 'viewer', 'active')`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	return db
}

func newKnowledgeGovernancePostgres(t *testing.T) (*gorm.DB, interfaces.KnowledgeGovernanceService) {
	t.Helper()
	db := newKnowledgeGovernancePostgresDatabase(t)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000082_knowledge_access_roles.up.sql")).Error)
	return db, NewKnowledgeGovernanceService(db, nil)
}

func createBusinessRoles(t *testing.T, service interfaces.KnowledgeGovernanceService, names ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(names))
	for _, name := range names {
		role, err := service.CreateBusinessRole(context.Background(), 1, name)
		require.NoError(t, err)
		ids = append(ids, role.ID)
	}
	return ids
}

func sameIDSet(actual []string, one []string, two []string) bool {
	contains := func(ids []string, id string) bool {
		for _, candidate := range ids {
			if candidate == id {
				return true
			}
		}
		return false
	}
	matches := func(expected []string) bool {
		if len(actual) != len(expected) {
			return false
		}
		for _, id := range actual {
			if !contains(expected, id) {
				return false
			}
		}
		return true
	}
	return matches(one) || matches(two)
}

// Whole-set replacements must serialize around their stable tenant member or KB lock;
// the observable result is one complete request, never a merged grant or assignment set.
func TestKnowledgeGovernancePostgresWholeSetReplacementSerialization(t *testing.T) {
	_, service := newKnowledgeGovernancePostgres(t)
	memberA := createBusinessRoles(t, service, "member A1", "member A2")
	memberB := createBusinessRoles(t, service, "member B1", "member B2")
	grantA := createBusinessRoles(t, service, "grant A1", "grant A2")
	grantB := createBusinessRoles(t, service, "grant B1", "grant B2")

	runPair := func(replace func([]string) error, first, second []string) {
		t.Helper()
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, set := range [][]string{first, second} {
			set := set
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results <- replace(set)
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		for err := range results {
			require.NoError(t, err)
		}
	}

	runPair(func(ids []string) error {
		return service.ReplaceMemberBusinessRoles(context.Background(), 1, "employee", ids)
	}, memberA, memberB)
	members, err := service.ListMemberBusinessRoleIDs(context.Background(), 1, "employee")
	require.NoError(t, err)
	require.True(t, sameIDSet(members, memberA, memberB), "member replacement was mixed: %v", members)

	runPair(func(ids []string) error {
		return service.ReplaceKnowledgeBaseRoleGrants(context.Background(), 1, "kb", "roles", ids)
	}, grantA, grantB)
	grants, err := service.GetKnowledgeBaseRoleGrants(context.Background(), 1, "kb")
	require.NoError(t, err)
	require.True(t, sameIDSet(grants, grantA, grantB), "KB replacement was mixed: %v", grants)
}

// A mutation that reached a role lock must re-read enabled state after the lock holder
// commits. It cannot resurrect a role assignment or access grant that was disabled meanwhile.
func TestKnowledgeGovernancePostgresDisableRejectsWaitingReplacement(t *testing.T) {
	db, service := newKnowledgeGovernancePostgres(t)
	disabled := createBusinessRoles(t, service, "to disable")[0]
	stable := createBusinessRoles(t, service, "stable")[0]
	require.NoError(t, service.ReplaceMemberBusinessRoles(context.Background(), 1, "employee", []string{stable}))
	require.NoError(t, service.ReplaceKnowledgeBaseRoleGrants(context.Background(), 1, "kb", "roles", []string{stable}))

	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, tx.Model(&types.BusinessRole{}).Where("tenant_id = ? AND id = ?", 1, disabled).Update("enabled", false).Error)
	memberResult := make(chan error, 1)
	grantResult := make(chan error, 1)
	go func() {
		memberResult <- service.ReplaceMemberBusinessRoles(context.Background(), 1, "employee", []string{disabled})
	}()
	go func() {
		grantResult <- service.ReplaceKnowledgeBaseRoleGrants(context.Background(), 1, "kb", "roles", []string{disabled})
	}()

	// Both requests are now blocked behind the role's transaction lock. On commit they
	// must see enabled=false, rather than validating the stale pre-lock read.
	select {
	case err := <-memberResult:
		t.Fatalf("member replacement did not wait for role lock: %v", err)
	case err := <-grantResult:
		t.Fatalf("grant replacement did not wait for role lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, tx.Commit().Error)
	for _, result := range []<-chan error{memberResult, grantResult} {
		select {
		case err := <-result:
			require.True(t, errors.Is(err, ErrKnowledgeAccessRoleInvalid), "unexpected replacement result: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("replacement did not finish after role lock was released")
		}
	}
	members, err := service.ListMemberBusinessRoleIDs(context.Background(), 1, "employee")
	require.NoError(t, err)
	require.Equal(t, []string{stable}, members)
	grants, err := service.GetKnowledgeBaseRoleGrants(context.Background(), 1, "kb")
	require.NoError(t, err)
	require.Equal(t, []string{stable}, grants)
}

func TestKnowledgeGovernancePostgresTombstoneRejoinDoesNotInheritRoles(t *testing.T) {
	db, service := newKnowledgeGovernancePostgres(t)
	role := createBusinessRoles(t, service, "rejoin role")[0]
	require.NoError(t, service.ReplaceMemberBusinessRoles(context.Background(), 1, "employee", []string{role}))
	require.NoError(t, db.Where("tenant_id = ? AND user_id = ?", 1, "employee").Delete(&types.TenantMember{}).Error)
	require.NoError(t, db.Create(&types.TenantMember{TenantID: 1, UserID: "employee", Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}).Error)
	ids, err := service.ListMemberBusinessRoleIDs(context.Background(), 1, "employee")
	require.NoError(t, err)
	require.Empty(t, ids)
}
