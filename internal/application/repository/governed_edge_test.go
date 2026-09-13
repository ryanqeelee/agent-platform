package repository

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func governedEdgeAuthority(t *testing.T, binding types.GovernedEdgeBinding) (interfaces.TenantRepository, uint64) {
	t.Helper()
	db := activationTestDB(t)
	require.NoError(t, db.AutoMigrate(&types.AuditLog{}))
	require.NoError(t, db.Omit("TenantID").Create(&types.User{
		ID: "system-admin", Username: "system-admin", Email: "system-admin@example.invalid",
		IsActive: true, IsSystemAdmin: true,
	}).Error)
	enterpriseID := binding.EnterpriseID
	tenant := &types.Tenant{
		Name: "Acme", Status: types.TenantStatusActive, AnalysisEnabled: true,
		GovernedEnterpriseID: &enterpriseID, GovernedEdgeBinding: &binding,
	}
	require.NoError(t, db.Create(tenant).Error)
	return NewTenantRepository(db), tenant.ID
}

func TestGovernedEdgePrepareConfirmAndReplayOwnRevision(t *testing.T) {
	ctx := context.Background()
	initial := types.GovernedEdgeBinding{
		BindingID: "binding", Revision: 7, DeploymentRevision: 4,
		EnterpriseID: "enterprise", EdgeNodeID: "edge", Enabled: false,
	}
	repo, tenantID := governedEdgeAuthority(t, initial)
	prepare := interfaces.GovernedEdgeBindingPrepareCommand{
		TenantID: tenantID, ActorUserID: "system-admin", ExpectedRevision: 7,
		EdgeNodeID: "edge", SourceID: "source", DeploymentRevision: 5,
	}
	prepared, err := repo.PrepareGovernedEdgeBinding(ctx, prepare)
	require.NoError(t, err)
	require.Equal(t, int64(8), prepared.Revision)
	require.False(t, prepared.Enabled)

	replayed, err := repo.PrepareGovernedEdgeBinding(ctx, prepare)
	require.NoError(t, err)
	require.Equal(t, *prepared, *replayed)

	confirm := interfaces.GovernedEdgeBindingConfirmCommand{
		TenantID: tenantID, ActorUserID: "system-admin", ExpectedRevision: prepared.Revision,
		EdgeNodeID: prepared.EdgeNodeID, SourceID: prepared.SourceID,
		DeploymentRevision: prepared.DeploymentRevision,
	}
	confirmed, err := repo.ConfirmGovernedEdgeBinding(ctx, confirm)
	require.NoError(t, err)
	require.True(t, confirmed.Enabled)
	require.Equal(t, int64(9), confirmed.Revision)

	replayedConfirm, err := repo.ConfirmGovernedEdgeBinding(ctx, confirm)
	require.NoError(t, err)
	require.Equal(t, *confirmed, *replayedConfirm)
}

func TestGovernedEdgeRevocationDistinguishesPreparedAndRevokedDisabledState(t *testing.T) {
	ctx := context.Background()
	prepared := types.GovernedEdgeBinding{
		BindingID: "binding", Revision: 8, DeploymentRevision: 5,
		EnterpriseID: "enterprise", EdgeNodeID: "edge", SourceID: "source", Enabled: false,
	}
	repo, tenantID := governedEdgeAuthority(t, prepared)

	receipt, err := repo.RevokeGovernedEdgeBinding(ctx, "enterprise", "edge", 5)
	require.NoError(t, err)
	require.Equal(t, "disabled", receipt.Status)
	require.Equal(t, int64(5), receipt.AcceptedControlRevision)
	require.Equal(t, int64(9), receipt.BindingRevision)

	replayed, err := repo.RevokeGovernedEdgeBinding(ctx, "enterprise", "edge", 5)
	require.NoError(t, err)
	require.Equal(t, *receipt, *replayed)

	_, err = repo.ConfirmGovernedEdgeBinding(ctx, interfaces.GovernedEdgeBindingConfirmCommand{
		TenantID: tenantID, ActorUserID: "system-admin", ExpectedRevision: 8,
		EdgeNodeID: "edge", SourceID: "source", DeploymentRevision: 5,
	})
	require.ErrorIs(t, err, ErrGovernedEdgeBindingConflict)
}

func TestGovernedEdgeRevocationReceiptUsesRealCurrentRevision(t *testing.T) {
	ctx := context.Background()
	current := types.GovernedEdgeBinding{
		BindingID: "binding", Revision: 12, DeploymentRevision: 9,
		EnterpriseID: "enterprise", EdgeNodeID: "current-edge", SourceID: "source", Enabled: true,
	}
	repo, _ := governedEdgeAuthority(t, current)

	differentNode, err := repo.RevokeGovernedEdgeBinding(ctx, "enterprise", "old-edge", 40)
	require.NoError(t, err)
	require.Equal(t, "superseded", differentNode.Status)
	require.Equal(t, int64(40), differentNode.SentControlRevision)
	require.Equal(t, int64(9), differentNode.AcceptedControlRevision)

	olderGeneration, err := repo.RevokeGovernedEdgeBinding(ctx, "enterprise", "current-edge", 8)
	require.NoError(t, err)
	require.Equal(t, "superseded", olderGeneration.Status)
	require.Equal(t, int64(9), olderGeneration.AcceptedControlRevision)

	_, err = repo.RevokeGovernedEdgeBinding(ctx, "enterprise", "current-edge", 9)
	require.ErrorIs(t, err, ErrGovernedEdgeBindingConflict)
}

func TestGovernedEdgeDelayedConfirmCannotUndoRevocationPostgres(t *testing.T) {
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	schema := "governed_edge_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
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
		`CREATE TABLE tenants (
			id bigint PRIMARY KEY, status text NOT NULL, analysis_enabled boolean NOT NULL,
			governed_enterprise_id text UNIQUE, governed_edge_binding jsonb,
			updated_at timestamptz, deleted_at timestamptz
		)`,
		`CREATE TABLE users (
			id text PRIMARY KEY, is_active boolean NOT NULL, is_system_admin boolean NOT NULL,
			tenant_id bigint, can_access_all_tenants boolean NOT NULL, deleted_at timestamptz
		)`,
		`CREATE TABLE audit_logs (
			id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, actor_user_id text,
			actor_role text, action text, scope_type text, scope_id text, target_type text,
			target_id text, target_user_id text, request_path text, request_method text,
			outcome text, details jsonb, created_at timestamptz
		)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	require.NoError(t, db.Exec(`INSERT INTO users
		(id,is_active,is_system_admin,tenant_id,can_access_all_tenants)
		VALUES ('system-admin',true,true,NULL,false)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tenants
		(id,status,analysis_enabled,governed_enterprise_id,governed_edge_binding)
		VALUES (1,'active',true,'enterprise',
		'{"binding_id":"binding","revision":8,"deployment_revision":5,"enterprise_id":"enterprise","edge_node_id":"edge","source_id":"source","enabled":false}')`).Error)

	tx := db.Begin()
	receipt, err := (&tenantRepository{db: tx}).RevokeGovernedEdgeBinding(context.Background(), "enterprise", "edge", 5)
	require.NoError(t, err)
	require.Equal(t, "disabled", receipt.Status)

	confirmResult := make(chan error, 1)
	go func() {
		_, confirmErr := NewTenantRepository(db).ConfirmGovernedEdgeBinding(context.Background(), interfaces.GovernedEdgeBindingConfirmCommand{
			TenantID: 1, ActorUserID: "system-admin", ExpectedRevision: 8,
			EdgeNodeID: "edge", SourceID: "source", DeploymentRevision: 5,
		})
		confirmResult <- confirmErr
	}()
	require.NoError(t, tx.Commit().Error)
	require.ErrorIs(t, <-confirmResult, ErrGovernedEdgeBindingConflict)
	var tenant types.Tenant
	require.NoError(t, db.First(&tenant, 1).Error)
	require.NotNil(t, tenant.GovernedEdgeBinding)
	require.False(t, tenant.GovernedEdgeBinding.Enabled)
	require.Empty(t, tenant.GovernedEdgeBinding.SourceID)
	require.Equal(t, int64(9), tenant.GovernedEdgeBinding.Revision)
}
