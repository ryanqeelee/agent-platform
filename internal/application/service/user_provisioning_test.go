package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type provisioningUserRepo struct {
	interfaces.UserRepository
	created       *types.User
	updatedTenant uint64
}

func (r *provisioningUserRepo) GetUserByEmail(context.Context, string) (*types.User, error) {
	return nil, nil
}

func (r *provisioningUserRepo) GetUserByUsername(context.Context, string) (*types.User, error) {
	return nil, nil
}

func (r *provisioningUserRepo) CreateUser(_ context.Context, user *types.User) error {
	copy := *user
	r.created = &copy
	return nil
}

func (r *provisioningUserRepo) UpdateUser(_ context.Context, user *types.User) error {
	r.updatedTenant = user.TenantID
	return nil
}

type provisioningTenantService struct {
	interfaces.TenantService
	createCalls int
}

func (s *provisioningTenantService) CreateTenant(context.Context, *types.Tenant) (*types.Tenant, error) {
	s.createCalls++
	return &types.Tenant{ID: 99}, nil
}

func (s *provisioningTenantService) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	return &types.Tenant{ID: id}, nil
}

type provisioningMemberService struct {
	interfaces.TenantMemberService
	members []*types.TenantMember
}

func (s *provisioningMemberService) ListByUser(context.Context, string) ([]*types.TenantMember, error) {
	return s.members, nil
}

func TestUserServiceRegisterTenantlessSkipsTenantCreation(t *testing.T) {
	repo := &provisioningUserRepo{}
	tenantSvc := &provisioningTenantService{}
	svc := &userService{userRepo: repo, tenantService: tenantSvc}

	user, err := svc.Register(context.Background(), &types.RegisterRequest{
		Username:           "alice",
		Email:              "alice@example.com",
		Password:           "supersecret",
		TenantProvisioning: types.TenantProvisioningTenantless,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if tenantSvc.createCalls != 0 {
		t.Fatalf("tenant create calls = %d, want 0", tenantSvc.createCalls)
	}
	if user.TenantID != 0 || repo.created == nil || repo.created.TenantID != 0 {
		t.Fatalf("tenantless user persisted with tenant: user=%d created=%v", user.TenantID, repo.created)
	}
}

func TestEnterpriseManagedLoginDoesNotRepairTenantlessUserFromMembership(t *testing.T) {
	repo := &provisioningUserRepo{}
	tenantSvc := &provisioningTenantService{}
	memberSvc := &provisioningMemberService{members: []*types.TenantMember{
		{TenantID: 42, Status: types.TenantMemberStatusActive},
	}}
	svc := &userService{userRepo: repo, tenantService: tenantSvc, memberService: memberSvc}
	user := &types.User{ID: "alice", TenantID: 0}

	if got := svc.resolveLoginTenantID(context.Background(), user); got != 0 {
		t.Fatalf("resolved tenant = %d, want tenantless", got)
	}
	if repo.updatedTenant != 0 || user.TenantID != 0 {
		t.Fatalf("historical membership rebound tenantless user: repo=%d user=%d", repo.updatedTenant, user.TenantID)
	}
}

func TestEnterpriseManagedSwitchTenantHonorsDisabledCrossTenantFlag(t *testing.T) {
	svc := &userService{config: &config.Config{Tenant: &config.TenantConfig{EnableCrossTenantAccess: false}}}
	user := &types.User{ID: "operator", TenantID: 1, CanAccessAllTenants: true}
	if _, err := svc.SwitchTenant(context.Background(), user, 2, ""); !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("cross-tenant switch with disabled flag: %v", err)
	}
}
