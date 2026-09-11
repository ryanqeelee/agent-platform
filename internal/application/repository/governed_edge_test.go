package repository

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGovernedEdgeBindingOwnsItsRevision(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	tenant := &types.Tenant{Name: "edge", Status: types.TenantStatusActive}
	require.NoError(t, db.Create(tenant).Error)
	repo := NewTenantRepository(db)
	binding := types.GovernedEdgeBinding{BindingID: "binding", EnterpriseID: "enterprise", EdgeNodeID: "edge", SourceID: "source", Revision: 1, Enabled: true}
	require.NoError(t, repo.ApplyGovernedEdgeBinding(ctx, tenant.ID, binding))
	stale, err := repo.GetTenantByID(ctx, tenant.ID)
	require.NoError(t, err)
	revoked := binding
	revoked.Revision, revoked.Enabled = 2, false
	require.NoError(t, repo.ApplyGovernedEdgeBinding(ctx, tenant.ID, revoked))
	require.NoError(t, repo.ApplyGovernedEdgeBinding(ctx, tenant.ID, revoked))
	require.Error(t, repo.ApplyGovernedEdgeBinding(ctx, tenant.ID, binding))
	conflicting := revoked
	conflicting.Enabled = true
	require.Error(t, repo.ApplyGovernedEdgeBinding(ctx, tenant.ID, conflicting))
	conflicting.Revision, conflicting.EnterpriseID = 3, "foreign"
	require.Error(t, repo.ApplyGovernedEdgeBinding(ctx, tenant.ID, conflicting))
	stale.Name = "renamed"
	require.NoError(t, repo.UpdateTenant(ctx, stale))
	current, err := repo.GetTenantByID(ctx, tenant.ID)
	require.NoError(t, err)
	require.Equal(t, revoked, *current.GovernedEdgeBinding)
	raw, err := json.Marshal(current)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "enterprise")
}

func TestGovernedEdgeDelayedEnableCannotUndoRevocationPostgres(t *testing.T) {
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	schema := "edge_" + uuid.NewString()[:8]
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE tenants (id bigint primary key, governed_edge_binding jsonb, updated_at timestamptz, deleted_at timestamptz)").Error)
	require.NoError(t, db.Exec("INSERT INTO tenants(id) VALUES (1)").Error)
	ctx := context.Background()
	binding := types.GovernedEdgeBinding{BindingID: "binding", EnterpriseID: "enterprise", EdgeNodeID: "edge", SourceID: "source", Revision: 3, Enabled: false}
	tx := db.Begin()
	require.NoError(t, (&tenantRepository{db: tx}).ApplyGovernedEdgeBinding(ctx, 1, binding))
	delayed := binding
	delayed.Revision, delayed.Enabled = 2, true
	result := make(chan error, 1)
	go func() { result <- NewTenantRepository(db).ApplyGovernedEdgeBinding(ctx, 1, delayed) }()
	require.NoError(t, tx.Commit().Error)
	require.Error(t, <-result)
	var current types.Tenant
	require.NoError(t, db.First(&current, 1).Error)
	require.Equal(t, binding, *current.GovernedEdgeBinding)
}
