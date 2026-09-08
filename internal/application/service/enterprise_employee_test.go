package service

import (
	"context"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"golang.org/x/crypto/bcrypt"
	"testing"
)

type employeeCapture struct {
	interfaces.TenantMemberService
	user *types.User
}

func (s *employeeCapture) CreateEmployee(_ context.Context, u *types.User) (*types.TenantMember, error) {
	s.user = u
	return &types.TenantMember{UserID: u.ID, TenantID: u.TenantID, Role: types.TenantRoleViewer}, nil
}
func TestEnterpriseEmployeePasswordAndIdentity(t *testing.T) {
	m := &employeeCapture{}
	s := &userService{memberService: m}
	password := " Employee123 "
	u, _, err := s.CreateEnterpriseEmployee(context.Background(), 42, &types.RegisterRequest{Username: " Alice ", Email: "ALICE@example.invalid", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if u.TenantID != 42 || u.Username != "Alice" || u.Email != "ALICE@example.invalid" || u.IsSystemAdmin || u.CanAccessAllTenants {
		t.Fatalf("unexpected identity: %+v", u.ToUserInfo())
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		t.Fatal("password altered")
	}
	m.user = nil
	if _, _, err := s.CreateEnterpriseEmployee(context.Background(), 42, &types.RegisterRequest{Password: "weak"}); err == nil || m.user != nil {
		t.Fatal("weak password reached persistence")
	}
}
