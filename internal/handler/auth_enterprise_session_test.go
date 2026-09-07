package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type enterpriseSessionUserService struct {
	interfaces.UserService
	user *types.User
}

func (s *enterpriseSessionUserService) GetCurrentUser(context.Context) (*types.User, error) {
	return s.user, nil
}

type enterpriseSessionMemberService struct {
	interfaces.TenantMemberService
	member *types.TenantMember
	err    error
}

func (s *enterpriseSessionMemberService) GetMembership(context.Context, string, uint64) (*types.TenantMember, error) {
	return s.member, s.err
}

func enterpriseSessionRouter(h *AuthHandler, user *types.User, activeTenantID uint64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.UserContextKey, user)
		ctx = context.WithValue(ctx, types.TenantIDContextKey, activeTenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/auth/enterprise-session", h.GetEnterpriseSession)
	return r
}

func TestEnterpriseSessionProjectionV1(t *testing.T) {
	tests := []struct {
		name         string
		role         types.TenantRole
		user         types.User
		activeTenant uint64
		wantStatus   int
		wantRole     string
		wantAdmin    bool
	}{
		{name: "owner", role: types.TenantRoleOwner, user: types.User{ID: "owner", TenantID: 7, IsActive: true}, activeTenant: 7, wantStatus: http.StatusForbidden},
		{name: "admin", role: types.TenantRoleAdmin, user: types.User{ID: "admin", TenantID: 7, IsActive: true}, activeTenant: 7, wantStatus: http.StatusOK, wantRole: "admin", wantAdmin: true},
		{name: "contributor maps to knowledge administrator", role: types.TenantRoleContributor, user: types.User{ID: "contributor", TenantID: 7, IsActive: true}, activeTenant: 7, wantStatus: http.StatusForbidden},
		{name: "viewer maps to employee", role: types.TenantRoleViewer, user: types.User{ID: "viewer", TenantID: 7, IsActive: true}, activeTenant: 7, wantStatus: http.StatusOK, wantRole: "employee", wantAdmin: false},
		{name: "inactive user rejected", role: types.TenantRoleOwner, user: types.User{ID: "inactive", TenantID: 7}, activeTenant: 7, wantStatus: http.StatusForbidden},
		{name: "system admin rejected", role: types.TenantRoleOwner, user: types.User{ID: "system", TenantID: 7, IsActive: true, IsSystemAdmin: true}, activeTenant: 7, wantStatus: http.StatusForbidden},
		{name: "cross tenant superuser rejected", role: types.TenantRoleOwner, user: types.User{ID: "super", TenantID: 7, IsActive: true, CanAccessAllTenants: true}, activeTenant: 7, wantStatus: http.StatusForbidden},
		{name: "active tenant differs from home", role: types.TenantRoleOwner, user: types.User{ID: "foreign", TenantID: 7, IsActive: true}, activeTenant: 8, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userService := &enterpriseSessionUserService{user: &tt.user}
			memberService := &enterpriseSessionMemberService{member: &types.TenantMember{UserID: tt.user.ID, TenantID: tt.activeTenant, Role: tt.role, Status: types.TenantMemberStatusActive}}
			h := &AuthHandler{userService: userService, tenantMemberSvc: memberService}
			req := httptest.NewRequest(http.MethodGet, "/auth/enterprise-session", nil)
			w := httptest.NewRecorder()
			enterpriseSessionRouter(h, &tt.user, tt.activeTenant).ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var got EnterpriseSessionProjectionV1
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Schema != "EnterpriseSessionProjectionV1" || got.ActorID != tt.user.ID || got.TenantID != 7 || got.Role != tt.wantRole || !got.Surfaces.EmployeeWorkspace || got.Surfaces.EnterpriseAdministration != tt.wantAdmin {
				t.Fatalf("unexpected projection: %+v", got)
			}
		})
	}
}

func TestEnterpriseSessionProjectionRejectsMissingOrInvalidMembership(t *testing.T) {
	user := &types.User{ID: "u1", TenantID: 7, IsActive: true}
	for _, member := range []*types.TenantMember{nil, {UserID: user.ID, TenantID: 7, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusSuspended}, {UserID: user.ID, TenantID: 7, Role: "invalid", Status: types.TenantMemberStatusActive}} {
		h := &AuthHandler{userService: &enterpriseSessionUserService{user: user}, tenantMemberSvc: &enterpriseSessionMemberService{member: member}}
		w := httptest.NewRecorder()
		enterpriseSessionRouter(h, user, 7).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/enterprise-session", nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("status=%d want 403 body=%s", w.Code, w.Body.String())
		}
	}
}
