package mcp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/stretchr/testify/require"
)

type fakeOAuthRepo struct {
	clients map[string]*types.MCPOAuthClient
	tokens  map[string]*types.MCPOAuthToken
}

type fakeCachedClient struct {
	serviceID   string
	connected   bool
	disconnects int
}

func (c *fakeCachedClient) Connect(context.Context) error { c.connected = true; return nil }
func (c *fakeCachedClient) Disconnect() error {
	c.connected = false
	c.disconnects++
	return nil
}
func (c *fakeCachedClient) Initialize(context.Context) (*InitializeResult, error) {
	return &InitializeResult{}, nil
}
func (c *fakeCachedClient) ListTools(context.Context) ([]*types.MCPTool, error) { return nil, nil }
func (c *fakeCachedClient) ListResources(context.Context) ([]*types.MCPResource, error) {
	return nil, nil
}
func (c *fakeCachedClient) CallTool(context.Context, string, map[string]interface{}) (*CallToolResult, error) {
	return nil, nil
}
func (c *fakeCachedClient) ReadResource(context.Context, string) (*ReadResourceResult, error) {
	return nil, nil
}
func (c *fakeCachedClient) IsConnected() bool    { return c.connected }
func (c *fakeCachedClient) GetServiceID() string { return c.serviceID }

func newFakeOAuthRepo() *fakeOAuthRepo {
	return &fakeOAuthRepo{
		clients: map[string]*types.MCPOAuthClient{},
		tokens:  map[string]*types.MCPOAuthToken{},
	}
}

func fakeOAuthKey(tenantID uint64, principal types.Principal, serviceID string) string {
	return fmt.Sprintf("%d|%s|%s", tenantID, principal.Normalize().StorageID(), serviceID)
}

func (r *fakeOAuthRepo) GetClient(
	_ context.Context, tenantID uint64, serviceID string,
) (*types.MCPOAuthClient, error) {
	return r.clients[fmt.Sprintf("%d|%s", tenantID, serviceID)], nil
}

func (r *fakeOAuthRepo) SaveClient(_ context.Context, client *types.MCPOAuthClient) error {
	r.clients[fmt.Sprintf("%d|%s", client.TenantID, client.ServiceID)] = client
	return nil
}

func (r *fakeOAuthRepo) DeleteClient(_ context.Context, tenantID uint64, serviceID string) error {
	delete(r.clients, fmt.Sprintf("%d|%s", tenantID, serviceID))
	return nil
}

func (r *fakeOAuthRepo) GetToken(
	ctx context.Context, tenantID uint64, userID, serviceID string,
) (*types.MCPOAuthToken, error) {
	return r.GetTokenForPrincipal(ctx, tenantID, types.Principal{Type: types.PrincipalWebUser, ID: userID}, serviceID)
}

func (r *fakeOAuthRepo) GetTokenForPrincipal(
	_ context.Context, tenantID uint64, principal types.Principal, serviceID string,
) (*types.MCPOAuthToken, error) {
	return r.tokens[fakeOAuthKey(tenantID, principal, serviceID)], nil
}

func (r *fakeOAuthRepo) SaveToken(_ context.Context, token *types.MCPOAuthToken) error {
	return r.SaveTokenForPrincipal(context.Background(), token)
}

func (r *fakeOAuthRepo) SaveTokenForPrincipal(_ context.Context, token *types.MCPOAuthToken) error {
	principal := types.Principal{Type: token.PrincipalType, ID: token.PrincipalID}.Normalize()
	if !principal.Valid() {
		principal = types.Principal{Type: types.PrincipalWebUser, ID: token.UserID}.Normalize()
	}
	r.tokens[fakeOAuthKey(token.TenantID, principal, token.ServiceID)] = token
	return nil
}

func (r *fakeOAuthRepo) DeleteToken(
	ctx context.Context, tenantID uint64, userID, serviceID string,
) error {
	return r.DeleteTokenForPrincipal(ctx, tenantID, types.Principal{Type: types.PrincipalWebUser, ID: userID}, serviceID)
}

func (r *fakeOAuthRepo) DeleteTokenForPrincipal(
	_ context.Context, tenantID uint64, principal types.Principal, serviceID string,
) error {
	delete(r.tokens, fakeOAuthKey(tenantID, principal, serviceID))
	return nil
}

func (r *fakeOAuthRepo) TryAcquireTokenRefreshLease(
	_ context.Context,
	tenantID uint64,
	principal types.Principal,
	serviceID, leaseID string,
	leaseUntil time.Time,
) (bool, error) {
	row := r.tokens[fakeOAuthKey(tenantID, principal, serviceID)]
	if row == nil || (row.RefreshLeaseUntil != nil && row.RefreshLeaseUntil.After(time.Now())) {
		return false, nil
	}
	row.RefreshLeaseID = leaseID
	row.RefreshLeaseUntil = &leaseUntil
	return true, nil
}

func (r *fakeOAuthRepo) ReleaseTokenRefreshLease(
	_ context.Context,
	tenantID uint64,
	principal types.Principal,
	serviceID, leaseID string,
) error {
	row := r.tokens[fakeOAuthKey(tenantID, principal, serviceID)]
	if row != nil && row.RefreshLeaseID == leaseID {
		row.RefreshLeaseID = ""
		row.RefreshLeaseUntil = nil
	}
	return nil
}

func TestDBTokenStoreUsesPrincipal(t *testing.T) {
	repo := newFakeOAuthRepo()
	principal := types.Principal{Type: types.PrincipalAPIExternalUser, ID: "7:external-42"}
	store := newDBTokenStore(repo, 7, principal, "svc-1")

	expiresAt := time.Now().Add(time.Hour).UTC()
	require.NoError(t, store.SaveToken(context.Background(), &transport.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		ExpiresAt:    expiresAt,
	}))

	row, err := repo.GetTokenForPrincipal(context.Background(), 7, principal, "svc-1")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, types.PrincipalAPIExternalUser, row.PrincipalType)
	require.Equal(t, "7:external-42", row.PrincipalID)
	require.Equal(t, principal.StorageID(), row.UserID)

	token, err := store.GetToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "access", token.AccessToken)
	require.Equal(t, "refresh", token.RefreshToken)
	require.Equal(t, expiresAt, token.ExpiresAt)
}

func TestManagedTokenStoreLeavesRefreshToOAuthRuntime(t *testing.T) {
	repo := newFakeOAuthRepo()
	principal := types.Principal{Type: types.PrincipalWebUser, ID: "user-1"}
	store := newManagedTokenStore(repo, 7, principal, "svc-1")
	require.NoError(t, store.SaveToken(context.Background(), &transport.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}))

	token, err := store.GetToken(context.Background())
	require.NoError(t, err)
	require.True(t, token.ExpiresAt.IsZero(), "mcp-go must not race WeKnora's coordinated refresh")
	row, err := repo.GetTokenForPrincipal(context.Background(), 7, principal, "svc-1")
	require.NoError(t, err)
	require.False(t, row.ExpiresAt.IsZero(), "the database must retain the real expiry for preflight checks")
}

func TestMCPClientCacheKeyIsolatesTenantAndOAuthPrincipal(t *testing.T) {
	service := &types.MCPService{
		ID:         "svc-1",
		AuthConfig: &types.MCPAuthConfig{AuthType: types.MCPAuthOAuth},
	}
	alice := types.Principal{Type: types.PrincipalAPIExternalUser, ID: "7:alice"}
	bob := types.Principal{Type: types.PrincipalAPIExternalUser, ID: "7:bob"}

	require.NotEqual(t, cacheKey(service, 7, alice), cacheKey(service, 7, bob))
	require.NotEqual(t, cacheKey(service, 7, alice), cacheKey(service, 8, alice))
	require.Contains(t, cacheKey(service, 7, alice), alice.ID)

	service.AuthConfig.AuthType = types.MCPAuthAPIKey
	require.Equal(t, "svc-1\x007", cacheKey(service, 7, alice))
	require.Equal(t, cacheKey(service, 7, alice), cacheKey(service, 7, bob))
	require.NotEqual(t, cacheKey(service, 7, alice), cacheKey(service, 8, alice))
}

func TestCloseClientInvalidatesEveryTenantAndPrincipalForGlobalService(t *testing.T) {
	manager := NewMCPManager(nil)
	t.Cleanup(manager.Shutdown)

	aliceTenant7 := &fakeCachedClient{serviceID: "svc-1", connected: true}
	aliceTenant8 := &fakeCachedClient{serviceID: "svc-1", connected: true}
	otherService := &fakeCachedClient{serviceID: "svc-10", connected: true}
	manager.clients["svc-1\x007\x00api_external_user\x007:alice"] = aliceTenant7
	manager.clients["svc-1\x008\x00api_external_user\x008:alice"] = aliceTenant8
	manager.clients["svc-10\x007"] = otherService

	require.NoError(t, manager.CloseClient("svc-1"))
	require.Equal(t, 1, aliceTenant7.disconnects)
	require.Equal(t, 1, aliceTenant8.disconnects)
	require.Equal(t, 0, otherService.disconnects)
	require.Empty(t, manager.clients["svc-1\x007\x00api_external_user\x007:alice"])
	require.Empty(t, manager.clients["svc-1\x008\x00api_external_user\x008:alice"])
	require.Same(t, otherService, manager.clients["svc-10\x007"])
}
