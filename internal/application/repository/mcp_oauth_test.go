package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMCPOAuthTestRepo(t *testing.T) *mcpOAuthRepository {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.MCPOAuthClient{}, &types.MCPOAuthToken{}))

	return NewMCPOAuthRepository(db).(*mcpOAuthRepository)
}

func TestMCPOAuthRepositoryTokenForPrincipalIsolated(t *testing.T) {
	repo := newMCPOAuthTestRepo(t)
	ctx := context.Background()
	expiresAt := time.Now().Add(time.Hour).UTC()

	webPrincipal := types.Principal{Type: types.PrincipalWebUser, ID: "u1"}
	apiPrincipal := types.Principal{Type: types.PrincipalAPIExternalUser, ID: "7:external-u1"}

	require.NoError(t, repo.SaveTokenForPrincipal(ctx, &types.MCPOAuthToken{
		TenantID:      7,
		UserID:        webPrincipal.StorageID(),
		PrincipalType: webPrincipal.Type,
		PrincipalID:   webPrincipal.ID,
		ServiceID:     "svc1",
		AccessToken:   "web-token",
		RefreshToken:  "web-refresh",
		TokenType:     "Bearer",
		ExpiresAt:     expiresAt,
	}))
	require.NoError(t, repo.SaveTokenForPrincipal(ctx, &types.MCPOAuthToken{
		TenantID:      7,
		UserID:        apiPrincipal.StorageID(),
		PrincipalType: apiPrincipal.Type,
		PrincipalID:   apiPrincipal.ID,
		ServiceID:     "svc1",
		AccessToken:   "api-token",
		RefreshToken:  "api-refresh",
		TokenType:     "Bearer",
		ExpiresAt:     expiresAt,
	}))

	webToken, err := repo.GetTokenForPrincipal(ctx, 7, webPrincipal, "svc1")
	require.NoError(t, err)
	require.Equal(t, "web-token", webToken.AccessToken)

	apiToken, err := repo.GetTokenForPrincipal(ctx, 7, apiPrincipal, "svc1")
	require.NoError(t, err)
	require.Equal(t, "api-token", apiToken.AccessToken)
}

func TestMCPOAuthRepositorySamePrincipalIsolatedAcrossTenants(t *testing.T) {
	repo := newMCPOAuthTestRepo(t)
	ctx := context.Background()
	principal := types.Principal{Type: types.PrincipalWebUser, ID: "u1"}

	require.NoError(t, repo.SaveClient(ctx, &types.MCPOAuthClient{
		TenantID: 7, ServiceID: "svc1", ClientID: "tenant-7-client",
	}))
	require.NoError(t, repo.SaveClient(ctx, &types.MCPOAuthClient{
		TenantID: 8, ServiceID: "svc1", ClientID: "tenant-8-client",
	}))
	for tenantID, accessToken := range map[uint64]string{7: "tenant-7-token", 8: "tenant-8-token"} {
		require.NoError(t, repo.SaveTokenForPrincipal(ctx, &types.MCPOAuthToken{
			TenantID:      tenantID,
			UserID:        principal.StorageID(),
			PrincipalType: principal.Type,
			PrincipalID:   principal.ID,
			ServiceID:     "svc1",
			AccessToken:   accessToken,
			ExpiresAt:     time.Now().Add(time.Hour).UTC(),
		}))
	}

	client7, err := repo.GetClient(ctx, 7, "svc1")
	require.NoError(t, err)
	client8, err := repo.GetClient(ctx, 8, "svc1")
	require.NoError(t, err)
	require.Equal(t, "tenant-7-client", client7.ClientID)
	require.Equal(t, "tenant-8-client", client8.ClientID)

	token7, err := repo.GetTokenForPrincipal(ctx, 7, principal, "svc1")
	require.NoError(t, err)
	token8, err := repo.GetTokenForPrincipal(ctx, 8, principal, "svc1")
	require.NoError(t, err)
	require.Equal(t, "tenant-7-token", token7.AccessToken)
	require.Equal(t, "tenant-8-token", token8.AccessToken)
}

func TestMCPOAuthRepositoryLegacyUserTokenUsesWebPrincipal(t *testing.T) {
	repo := newMCPOAuthTestRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.SaveToken(ctx, &types.MCPOAuthToken{
		TenantID:     7,
		UserID:       "u1",
		ServiceID:    "svc1",
		AccessToken:  "legacy-token",
		RefreshToken: "legacy-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour).UTC(),
	}))

	token, err := repo.GetTokenForPrincipal(ctx, 7, types.Principal{Type: types.PrincipalWebUser, ID: "u1"}, "svc1")
	require.NoError(t, err)
	require.Equal(t, "legacy-token", token.AccessToken)
}

func TestMCPOAuthRepositoryRefreshLeaseHasSingleOwner(t *testing.T) {
	repo := newMCPOAuthTestRepo(t)
	ctx := context.Background()
	principal := types.Principal{Type: types.PrincipalWebUser, ID: "u1"}
	require.NoError(t, repo.SaveTokenForPrincipal(ctx, &types.MCPOAuthToken{
		TenantID:      7,
		UserID:        principal.StorageID(),
		PrincipalType: principal.Type,
		PrincipalID:   principal.ID,
		ServiceID:     "svc1",
		AccessToken:   "access",
		RefreshToken:  "refresh",
		ExpiresAt:     time.Now().Add(-time.Minute),
	}))

	first, err := repo.TryAcquireTokenRefreshLease(
		ctx, 7, principal, "svc1", "lease-1", time.Now().Add(time.Minute),
	)
	require.NoError(t, err)
	require.True(t, first)

	second, err := repo.TryAcquireTokenRefreshLease(
		ctx, 7, principal, "svc1", "lease-2", time.Now().Add(time.Minute),
	)
	require.NoError(t, err)
	require.False(t, second)

	// A non-owner cannot release the current owner's lease.
	require.NoError(t, repo.ReleaseTokenRefreshLease(ctx, 7, principal, "svc1", "lease-2"))
	second, err = repo.TryAcquireTokenRefreshLease(
		ctx, 7, principal, "svc1", "lease-2", time.Now().Add(time.Minute),
	)
	require.NoError(t, err)
	require.False(t, second)

	require.NoError(t, repo.ReleaseTokenRefreshLease(ctx, 7, principal, "svc1", "lease-1"))
	second, err = repo.TryAcquireTokenRefreshLease(
		ctx, 7, principal, "svc1", "lease-2", time.Now().Add(time.Minute),
	)
	require.NoError(t, err)
	require.True(t, second)
}

func TestGlobalMCPMigrationPreservesServiceApprovalAndOAuthReferences(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_mcp_global_")
	require.NoError(t, db.Exec(`CREATE TABLE mcp_services (
		id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, name varchar(255) NOT NULL,
		transport_type varchar(50) NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE INDEX idx_mcp_services_tenant_id ON mcp_services(tenant_id)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE mcp_tool_approvals (
		id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, service_id varchar(36) NOT NULL,
		tool_name varchar(512) NOT NULL, require_approval boolean NOT NULL DEFAULT false,
		CONSTRAINT fk_approval_service FOREIGN KEY(service_id) REFERENCES mcp_services(id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_mcp_tool_approvals_tenant_svc_tool
		ON mcp_tool_approvals(tenant_id, service_id, tool_name)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE mcp_oauth_clients (
		id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, service_id varchar(36) NOT NULL,
		client_id varchar(512) NOT NULL,
		CONSTRAINT fk_client_service FOREIGN KEY(service_id) REFERENCES mcp_services(id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE mcp_oauth_tokens (
		id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, principal_type varchar(32) NOT NULL,
		principal_id varchar(512) NOT NULL, service_id varchar(36) NOT NULL,
		CONSTRAINT fk_token_service FOREIGN KEY(service_id) REFERENCES mcp_services(id)
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO mcp_services(id, tenant_id, name, transport_type)
		VALUES ('svc-1', 7, 'service', 'http-streamable')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO mcp_tool_approvals(id, tenant_id, service_id, tool_name)
		VALUES ('approval-1', 7, 'svc-1', 'write')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO mcp_oauth_clients(id, tenant_id, service_id, client_id)
		VALUES ('client-1', 7, 'svc-1', 'client')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO mcp_oauth_tokens(id, tenant_id, principal_type, principal_id, service_id)
		VALUES ('token-1', 7, 'web_user', 'u1', 'svc-1')`).Error)

	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000111_global_mcp_services.up.sql")).Error)
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM mcp_services WHERE id = 'svc-1'`).Scan(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, db.Raw(`SELECT count(*) FROM mcp_tool_approvals
		WHERE id = 'approval-1' AND service_id = 'svc-1'`).Scan(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, db.Raw(`SELECT count(*) FROM mcp_oauth_clients
		WHERE id = 'client-1' AND tenant_id = 7 AND service_id = 'svc-1'`).Scan(&count).Error)
	require.Equal(t, int64(1), count)
	require.NoError(t, db.Raw(`SELECT count(*) FROM mcp_oauth_tokens
		WHERE id = 'token-1' AND tenant_id = 7 AND principal_id = 'u1' AND service_id = 'svc-1'`).Scan(&count).Error)
	require.Equal(t, int64(1), count)

	for _, table := range []string{"mcp_services", "mcp_tool_approvals"} {
		require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.columns
			WHERE table_name = ? AND column_name = 'tenant_id'`, table).Scan(&count).Error)
		require.Zero(t, count)
	}
}
