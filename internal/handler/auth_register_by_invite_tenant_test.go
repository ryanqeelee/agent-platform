package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type invitedRegistrationUserService struct {
	interfaces.UserService
	registeredMode        types.TenantProvisioningMode
	deleteTenantlessCalls int
}

func (s *invitedRegistrationUserService) GetUserByEmail(context.Context, string) (*types.User, error) {
	return nil, nil
}

func (s *invitedRegistrationUserService) Register(_ context.Context, req *types.RegisterRequest) (*types.User, error) {
	s.registeredMode = req.TenantProvisioning
	return &types.User{ID: "new-user", Username: req.Username, Email: req.Email, IsActive: true}, nil
}

func (s *invitedRegistrationUserService) GenerateTokens(context.Context, *types.User) (string, string, error) {
	return "access", "refresh", nil
}

func (s *invitedRegistrationUserService) DeleteTenantlessUser(context.Context, string) error {
	s.deleteTenantlessCalls++
	return nil
}

type invitedRegistrationInvitationService struct {
	interfaces.TenantInvitationService
	acceptErr error
}

func (s *invitedRegistrationInvitationService) LookupByToken(context.Context, string) (*types.TenantInvitation, error) {
	return &types.TenantInvitation{TenantID: 42, Role: types.TenantRoleViewer}, nil
}

func (s *invitedRegistrationInvitationService) AcceptByToken(context.Context, string, string) (*types.TenantMember, error) {
	if s.acceptErr != nil {
		return nil, s.acceptErr
	}
	return &types.TenantMember{TenantID: 42, Role: types.TenantRoleViewer}, nil
}

type invitedRegistrationTenantService struct {
	interfaces.TenantService
}

func TestRegisterByInviteRestoresTenantlessAccountWhenInviteExpiresDuringRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &invitedRegistrationUserService{}
	h := &AuthHandler{
		userService:   users,
		tenantService: &invitedRegistrationTenantService{},
		invitationSvc: &invitedRegistrationInvitationService{acceptErr: service.ErrInvitationTokenInvalid},
	}
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/auth/register-by-invite", h.RegisterByInvite)

	body := []byte(`{"token":"invite-token","email":"alice@example.com","username":"alice","password":"supersecret"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/register-by-invite", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if users.deleteTenantlessCalls != 1 {
		t.Fatalf("DeleteTenantlessUser called %d times, want 1", users.deleteTenantlessCalls)
	}
}

func TestRegisterByInviteReportsUnexpectedAcceptanceFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &invitedRegistrationUserService{}
	h := &AuthHandler{
		userService:   users,
		tenantService: &invitedRegistrationTenantService{},
		invitationSvc: &invitedRegistrationInvitationService{acceptErr: fmt.Errorf("database unavailable")},
	}
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/auth/register-by-invite", h.RegisterByInvite)

	body := []byte(`{"token":"invite-token","email":"alice@example.com","username":"alice","password":"supersecret"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/register-by-invite", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if users.deleteTenantlessCalls != 1 {
		t.Fatalf("DeleteTenantlessUser called %d times, want 1", users.deleteTenantlessCalls)
	}
}

func (s *invitedRegistrationTenantService) GetTenantByID(context.Context, uint64) (*types.Tenant, error) {
	return &types.Tenant{ID: 42, Name: "Invited Workspace"}, nil
}

func TestRegisterByInviteUsesInvitedTenantWithoutPersonalTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &invitedRegistrationUserService{}
	h := &AuthHandler{
		userService:   users,
		tenantService: &invitedRegistrationTenantService{},
		invitationSvc: &invitedRegistrationInvitationService{},
	}
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/auth/register-by-invite", h.RegisterByInvite)

	body := []byte(`{"token":"invite-token","email":"alice@example.com","username":"alice","password":"supersecret"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/register-by-invite", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if users.registeredMode != types.TenantProvisioningTenantless {
		t.Fatalf("register mode=%q, want tenantless", users.registeredMode)
	}
}
