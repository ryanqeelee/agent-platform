package router

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const invitationIdentityJWTSecret = "router-invitation-identity-test-secret"

func init() {
	_ = os.Setenv("JWT_SECRET", invitationIdentityJWTSecret)
}

type invitationIdentityTenantService struct {
	interfaces.TenantService
	tenant *types.Tenant
	calls  int
}

func (s *invitationIdentityTenantService) GetTenantByID(context.Context, uint64) (*types.Tenant, error) {
	s.calls++
	return s.tenant, nil
}

func signedInvitationIdentityJWT(t *testing.T, expiresAt time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "existing-user", "tenant_id": float64(1), "type": "access",
		"exp": expiresAt.Unix(),
	})
	signed, err := token.SignedString([]byte(invitationIdentityJWTSecret))
	require.NoError(t, err)
	return signed
}

func TestInvitationAcceptanceUsesIdentityJWTAndDefersInactiveTenantToRepository(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := filepath.Join(t.TempDir(), "invitation-identity.db") + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{}, &types.User{}, &types.AuthToken{},
		&types.TenantMember{}, &types.TenantInvitation{},
	))
	tenant := &types.Tenant{ID: 1, Name: "Suspended", Status: types.TenantStatusSuspended}
	user := &types.User{
		ID: "existing-user", Username: "existing-user", Email: "existing@example.invalid",
		PasswordHash: "unused", TenantID: tenant.ID, IsActive: true,
	}
	require.NoError(t, db.Create(tenant).Error)
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&types.TenantMember{
		UserID: user.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer,
		Status: types.TenantMemberStatusActive, JoinedAt: time.Now(),
	}).Error)
	direct := &types.TenantInvitation{
		TenantID: tenant.ID, InviteeUserID: user.ID, Role: types.TenantRoleViewer,
		Status: types.TenantInvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}
	share := &types.TenantInvitation{
		TenantID: tenant.ID, Token: "share-token", Role: types.TenantRoleViewer,
		Status: types.TenantInvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, db.Create(direct).Error)
	require.NoError(t, db.Create(share).Error)

	accessJWT := signedInvitationIdentityJWT(t, time.Now().Add(time.Hour))
	require.NoError(t, db.Create(&types.AuthToken{
		ID: uuid.NewString(), UserID: user.ID, Token: accessJWT,
		TokenType: "access_token", ExpiresAt: time.Now().Add(time.Hour),
	}).Error)
	tenantService := &invitationIdentityTenantService{tenant: tenant}
	userService := service.NewUserService(
		nil, apprepo.NewUserRepository(db), apprepo.NewAuthTokenRepository(db), tenantService, nil, nil,
	)
	invitationService := service.NewTenantInvitationService(apprepo.NewTenantInvitationRepository(db), nil, nil)
	invitationHandler := handler.NewTenantInvitationHandler(
		invitationService, userService, tenantService, nil, nil, nil,
	)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(middleware.Auth(tenantService, userService, nil, nil, nil))
	v1 := engine.Group("/api/v1")
	RegisterMyInvitationRoutes(v1, invitationHandler)
	otherTenantHandlerCalled := false
	v1.GET("/knowledge-bases", func(c *gin.Context) {
		otherTenantHandlerCalled = true
		c.Status(http.StatusNoContent)
	})

	requests := []struct {
		name string
		path string
		body []byte
	}{
		{name: "direct", path: "/api/v1/me/invitations/" + fmt.Sprint(direct.ID) + "/accept"},
		{name: "share", path: "/api/v1/me/invitations/accept-by-token", body: []byte(`{"token":"share-token"}`)},
	}
	for _, tc := range requests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(tc.body))
			request.Header.Set("Authorization", "Bearer "+accessJWT)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
		})
	}
	require.Equal(t, 0, tenantService.calls, "identity-only acceptance must not resolve the current tenant")
	for _, invitationID := range []uint64{direct.ID, share.ID} {
		var stored types.TenantInvitation
		require.NoError(t, db.First(&stored, invitationID).Error)
		require.Equal(t, types.TenantInvitationStatusPending, stored.Status)
		require.Equal(t, 0, stored.AcceptedCount)
		require.Nil(t, stored.RespondedAt)
	}

	for name, token := range map[string]string{
		"expired": signedInvitationIdentityJWT(t, time.Now().Add(-time.Hour)),
		"invalid": accessJWT + "tampered",
	} {
		t.Run(name+" JWT", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/me/invitations/"+fmt.Sprint(direct.ID)+"/accept", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
		})
	}

	for _, path := range []string{
		"/api/v1/me/invitations",
		"/api/v1/me/invitations/" + fmt.Sprint(direct.ID) + "/decline",
		"/api/v1/knowledge-bases",
	} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/decline") {
			method = http.MethodPost
		}
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+accessJWT)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusUnauthorized, response.Code, path+": "+response.Body.String())
	}
	require.False(t, otherTenantHandlerCalled)
}
