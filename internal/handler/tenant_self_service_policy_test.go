package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

func tenantPolicyErrorCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		if appErr, ok := c.Errors.Last().Err.(*apperrors.AppError); ok {
			c.JSON(appErr.HTTPCode, gin.H{"error": appErr})
		}
	}
}

type tenantPolicySettingService struct {
	interfaces.SystemSettingService
	enabled bool
}

func (s *tenantPolicySettingService) GetBool(context.Context, string, string, bool) bool {
	return s.enabled
}

func (s *tenantPolicySettingService) GetString(_ context.Context, _ string, _ string, def string) string {
	return def
}

func (s *tenantPolicySettingService) GetInt(_ context.Context, _ string, _ string, def int64) int64 {
	return def
}

type tenantPolicyUserService struct {
	interfaces.UserService
	user *types.User
}

func (s *tenantPolicyUserService) GetCurrentUser(context.Context) (*types.User, error) {
	return s.user, nil
}

func (s *tenantPolicyUserService) BuildLoginMemberships(context.Context, *types.User, *types.Tenant) []types.Membership {
	return []types.Membership{}
}

type tenantPolicyTenantService struct {
	interfaces.TenantService
	createCalls int
}

type tenantPolicyMemberService struct {
	interfaces.TenantMemberService
	ensureOwnerCalls int
	member           *types.TenantMember
}

func (s *tenantPolicyMemberService) EnsureAdministrator(context.Context, string, uint64) (*types.TenantMember, error) {
	s.ensureOwnerCalls++
	return nil, errors.New("catalog manager must not become tenant owner")
}

func (s *tenantPolicyMemberService) GetMembership(context.Context, string, uint64) (*types.TenantMember, error) {
	return s.member, nil
}

func (s *tenantPolicyTenantService) CreateTenant(_ context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	s.createCalls++
	tenant.ID = 99
	return tenant, nil
}

func (s *tenantPolicyTenantService) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	return &types.Tenant{ID: id, Name: "workspace"}, nil
}

func TestCreateTenantRejectsRegularUserWhenSelfServiceDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenants := &tenantPolicyTenantService{}
	h := &TenantHandler{
		service:          tenants,
		userService:      &tenantPolicyUserService{user: &types.User{ID: "regular-user"}},
		config:           &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.Use(tenantPolicyErrorCapture())
	r.POST("/tenants", h.CreateTenant)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"blocked"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tenants.createCalls != 0 {
		t.Fatalf("CreateTenant called %d times, want 0", tenants.createCalls)
	}
	if !strings.Contains(w.Body.String(), `"code":2005`) {
		t.Fatalf("response missing typed disabled code: %s", w.Body.String())
	}
}

func TestCreateTenantRejectsDormantCrossTenantSuperuserWhenFlagDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenants := &tenantPolicyTenantService{}
	h := &TenantHandler{
		service: tenants,
		userService: &tenantPolicyUserService{user: &types.User{
			ID:                  "super-user",
			TenantID:            1,
			CanAccessAllTenants: true,
		}},
		config:           &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/tenants", h.CreateTenant)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"admin-created"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tenants.createCalls != 0 {
		t.Fatalf("CreateTenant called %d times, want 0", tenants.createCalls)
	}
}

func TestCreateTenantAllowsCrossTenantSuperuserWhenFlagEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenants := &tenantPolicyTenantService{}
	members := &tenantPolicyMemberService{}
	h := &TenantHandler{
		service:       tenants,
		memberService: members,
		userService: &tenantPolicyUserService{user: &types.User{
			ID:                  "super-user",
			TenantID:            1,
			CanAccessAllTenants: true,
		}},
		config:           &config.Config{Tenant: &config.TenantConfig{EnableCrossTenantAccess: true}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/tenants", h.CreateTenant)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants", bytes.NewBufferString(`{"name":"admin-created"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if tenants.createCalls != 1 {
		t.Fatalf("CreateTenant called %d times, want 1", tenants.createCalls)
	}
	if members.ensureOwnerCalls != 0 {
		t.Fatalf("EnsureAdministrator called %d times, want 0", members.ensureOwnerCalls)
	}
}

func TestAuthMeProjectsTenantCreationCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{
		userService: &tenantPolicyUserService{user: &types.User{
			ID:                  "tenantless-user",
			Username:            "tenantless",
			Email:               "tenantless@example.com",
			CanAccessAllTenants: true,
		}},
		configInfo:       &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: false},
	}
	r := gin.New()
	r.GET("/auth/me", h.GetCurrentUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"can_create_tenant":false`) {
		t.Fatalf("response missing capability: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"can_manage_all_tenant_members":false`) {
		t.Fatalf("response leaked disabled cross-tenant member authority: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"can_access_all_tenants":true`) {
		t.Fatalf("response masked stored cross-tenant privilege: %s", w.Body.String())
	}
}

func TestAuthMeDoesNotAdvertiseTenantCreationToViewer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{
		userService:      &tenantPolicyUserService{user: &types.User{ID: "viewer", TenantID: 7}},
		tenantService:    &tenantPolicyTenantService{},
		configInfo:       &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{enabled: true},
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(7))
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleViewer)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/auth/me", h.GetCurrentUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"can_create_tenant":false`) {
		t.Fatalf("viewer response advertised tenant creation: %s", w.Body.String())
	}
}

func TestAuthMeProjectsOperatingAnalysisAccessFromPersistedMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{
		userService:   &tenantPolicyUserService{user: &types.User{ID: "viewer", TenantID: 7, IsActive: true}},
		tenantService: &tenantPolicyTenantService{},
		tenantMemberSvc: &tenantPolicyMemberService{member: &types.TenantMember{
			UserID: "viewer", TenantID: 7, Status: types.TenantMemberStatusActive,
			OperatingAnalysisAccess: true,
		}},
		configInfo:       &config.Config{Tenant: &config.TenantConfig{}},
		systemSettingSvc: &tenantPolicySettingService{},
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(7))
		ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleViewer)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/auth/me", h.GetCurrentUser)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/me", nil))

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"operating_analysis_access":{"schema":"OperatingAnalysisAccessV1","tenant_id":7,"membership_status":"active","enabled":true}`) {
		t.Fatalf("unexpected strict access projection: status=%d body=%s", w.Code, w.Body.String())
	}
}
