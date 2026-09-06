package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type authTenantService struct {
	interfaces.TenantService
	status string
}

func (s *authTenantService) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	return &types.Tenant{ID: id, Status: s.status}, nil
}

func init() {
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-user-auth-token-tests")
}

type stubAuthTokenRepo struct {
	tokens         map[string]*types.AuthToken
	revokedUserIDs []string
}

func (s *stubAuthTokenRepo) CreateToken(context.Context, *types.AuthToken) error { return nil }
func (s *stubAuthTokenRepo) GetTokenByValue(_ context.Context, tokenValue string) (*types.AuthToken, error) {
	token, ok := s.tokens[tokenValue]
	if !ok {
		return nil, errors.New("token not found")
	}
	return token, nil
}
func (s *stubAuthTokenRepo) GetTokensByUserID(context.Context, string) ([]*types.AuthToken, error) {
	return nil, nil
}
func (s *stubAuthTokenRepo) UpdateToken(context.Context, *types.AuthToken) error { return nil }
func (s *stubAuthTokenRepo) DeleteToken(context.Context, string) error           { return nil }
func (s *stubAuthTokenRepo) DeleteExpiredTokens(context.Context) error           { return nil }
func (s *stubAuthTokenRepo) RevokeTokensByUserID(_ context.Context, userID string) error {
	s.revokedUserIDs = append(s.revokedUserIDs, userID)
	return nil
}

type stubUserRepoForAuth struct {
	users       map[string]*types.User
	updateCalls int
}

func (s *stubUserRepoForAuth) CreateUser(context.Context, *types.User) error { return nil }
func (s *stubUserRepoForAuth) GetUserByID(_ context.Context, id string) (*types.User, error) {
	user, ok := s.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return user, nil
}
func (s *stubUserRepoForAuth) GetUsersByIDs(context.Context, []string) (map[string]*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByEmail(context.Context, string) (*types.User, error) {
	for _, user := range s.users {
		if user.Email != "" {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
}
func (s *stubUserRepoForAuth) GetUserByUsername(context.Context, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) GetUserByTenantID(context.Context, uint64) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) UpdateUser(context.Context, *types.User) error {
	s.updateCalls++
	return nil
}
func (s *stubUserRepoForAuth) DeleteUser(context.Context, string) error { return nil }
func (s *stubUserRepoForAuth) ListUsers(context.Context, int, int) ([]*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) ListSystemAdmins(context.Context, int, int) ([]*types.User, int64, error) {
	return nil, 0, nil
}
func (s *stubUserRepoForAuth) RevokeSystemAdmin(context.Context, string, string) (*types.User, error) {
	return nil, nil
}
func (s *stubUserRepoForAuth) SearchUsers(context.Context, string, int) ([]*types.User, error) {
	return nil, nil
}

func newAuthTestUserService(tokenRepo *stubAuthTokenRepo) *userService {
	return &userService{
		userRepo: &stubUserRepoForAuth{
			users: map[string]*types.User{
				"user-1": {ID: "user-1", TenantID: 1, IsActive: true},
			},
		},
		tokenRepo:     tokenRepo,
		tenantService: &authTenantService{status: types.TenantStatusActive},
	}
}

func signTestJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(getJwtSecret()))
	if err != nil {
		panic(err)
	}
	return signed
}

func TestValidateTokenRejectsRefreshToken(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	refreshJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "refresh",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[refreshJWT] = &types.AuthToken{
		UserID:    "user-1",
		Token:     refreshJWT,
		TokenType: "refresh_token",
	}

	_, _, err := svc.ValidateToken(ctx, refreshJWT)
	if err == nil || err.Error() != "refresh token cannot be used as access token" {
		t.Fatalf("ValidateToken(refresh JWT) err = %v, want refresh rejection", err)
	}

	legacyRefresh := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[legacyRefresh] = &types.AuthToken{
		UserID:    "user-1",
		Token:     legacyRefresh,
		TokenType: "refresh_token",
	}

	_, _, err = svc.ValidateToken(ctx, legacyRefresh)
	if err == nil || err.Error() != "refresh token cannot be used as access token" {
		t.Fatalf("ValidateToken(legacy refresh in DB) err = %v, want refresh rejection", err)
	}
}

func TestValidateTokenRejectsInactiveUser(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	svc.userRepo.(*stubUserRepoForAuth).users["user-1"].IsActive = false

	accessJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "access",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[accessJWT] = &types.AuthToken{
		UserID:    "user-1",
		Token:     accessJWT,
		TokenType: "access_token",
	}

	if _, _, err := svc.ValidateToken(ctx, accessJWT); err == nil || err.Error() != "account is disabled" {
		t.Fatalf("ValidateToken(inactive user) err = %v, want account disabled rejection", err)
	}
}

func TestRefreshTokenRejectsAccessTokenRecord(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	refreshJWT := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "refresh",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenRepo.tokens[refreshJWT] = &types.AuthToken{
		UserID:    "user-1",
		Token:     refreshJWT,
		TokenType: "access_token",
	}

	_, _, err := svc.RefreshToken(ctx, refreshJWT)
	if err == nil || err.Error() != "not a refresh token" {
		t.Fatalf("RefreshToken(access token record) err = %v, want not a refresh token", err)
	}
}

func suspendedAuthTestService(t *testing.T) (*userService, *stubAuthTokenRepo) {
	t.Helper()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	user := svc.userRepo.(*stubUserRepoForAuth).users["user-1"]
	user.Email = "suspended@example.invalid"
	hash, err := bcrypt.GenerateFromPassword([]byte("CorrectHorse9"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user.PasswordHash = string(hash)
	members, repo := newServiceWithRepo()
	repo.rows = []*types.TenantMember{{
		UserID: user.ID, TenantID: user.TenantID, Role: types.TenantRoleViewer,
		Status: types.TenantMemberStatusSuspended,
	}}
	svc.memberService = members
	return svc, tokenRepo
}

func TestTokenIssuanceRejectsSuspendedMembershipAcrossEntryPoints(t *testing.T) {
	t.Run("password login", func(t *testing.T) {
		svc, _ := suspendedAuthTestService(t)
		resp, err := svc.Login(context.Background(), &types.LoginRequest{
			Email: "suspended@example.invalid", Password: "CorrectHorse9",
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.Success || resp.Token != "" || resp.RefreshToken != "" {
			t.Fatalf("suspended login issued credentials: %+v", resp)
		}
		if resp.Message != "Workspace membership is suspended" {
			t.Fatalf("suspended login message = %q", resp.Message)
		}
	})

	t.Run("refresh", func(t *testing.T) {
		svc, tokenRepo := suspendedAuthTestService(t)
		refreshJWT := signTestJWT(jwt.MapClaims{
			"user_id": "user-1", "type": "refresh", "exp": time.Now().Add(time.Hour).Unix(),
		})
		tokenRepo.tokens[refreshJWT] = &types.AuthToken{
			UserID: "user-1", Token: refreshJWT, TokenType: "refresh_token",
		}
		access, rotated, err := svc.RefreshToken(context.Background(), refreshJWT)
		if !errors.Is(err, ErrMembershipSuspended) || access != "" || rotated != "" {
			t.Fatalf("suspended refresh = (%q, %q, %v), want rejection", access, rotated, err)
		}
	})

	t.Run("Lite AutoSetup issuance seam", func(t *testing.T) {
		svc, _ := suspendedAuthTestService(t)
		access, refresh, err := svc.GenerateTokens(
			context.Background(), svc.userRepo.(*stubUserRepoForAuth).users["user-1"])
		if !errors.Is(err, ErrMembershipSuspended) || access != "" || refresh != "" {
			t.Fatalf("suspended AutoSetup issuance = (%q, %q, %v), want rejection", access, refresh, err)
		}
	})
}

func TestActivationTenantStatusGatesLoginRefreshAndJWTValidation(t *testing.T) {
	for _, status := range []string{
		types.TenantStatusProvisioning,
		types.TenantStatusActivationAbandoned,
		types.TenantStatusActive,
	} {
		t.Run(status, func(t *testing.T) {
			tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
			svc := newAuthTestUserService(tokenRepo)
			svc.tenantService = &authTenantService{status: status}
			user := svc.userRepo.(*stubUserRepoForAuth).users["user-1"]
			user.Email = "owner@example.invalid"
			hash, err := bcrypt.GenerateFromPassword([]byte("CorrectHorse9"), bcrypt.MinCost)
			require.NoError(t, err)
			user.PasswordHash = string(hash)
			members, repo := newServiceWithRepo()
			repo.rows = []*types.TenantMember{{
				UserID: user.ID, TenantID: user.TenantID, Role: types.TenantRoleOwner,
				Status: types.TenantMemberStatusActive,
			}}
			svc.memberService = members

			login, err := svc.Login(context.Background(), &types.LoginRequest{
				Email: user.Email, Password: "CorrectHorse9",
			})
			require.NoError(t, err)
			if status == types.TenantStatusActive {
				require.True(t, login.Success)
				require.NotEmpty(t, login.Token)
			} else {
				require.False(t, login.Success)
				require.Empty(t, login.Token)
			}

			refreshJWT := signTestJWT(jwt.MapClaims{
				"user_id": user.ID, "type": "refresh", "exp": time.Now().Add(time.Hour).Unix(),
			})
			tokenRepo.tokens[refreshJWT] = &types.AuthToken{
				UserID: user.ID, Token: refreshJWT, TokenType: "refresh_token",
			}
			access, rotated, refreshErr := svc.RefreshToken(context.Background(), refreshJWT)
			if status == types.TenantStatusActive {
				require.NoError(t, refreshErr)
				require.NotEmpty(t, access)
				require.NotEmpty(t, rotated)
			} else {
				require.ErrorIs(t, refreshErr, ErrTenantNotActive)
				require.Empty(t, access)
				require.Empty(t, rotated)
			}

			accessJWT := signTestJWT(jwt.MapClaims{
				"user_id": user.ID, "tenant_id": user.TenantID, "type": "access", "exp": time.Now().Add(time.Hour).Unix(),
			})
			tokenRepo.tokens[accessJWT] = &types.AuthToken{
				UserID: user.ID, Token: accessJWT, TokenType: "access_token",
			}
			_, _, validateErr := svc.ValidateToken(context.Background(), accessJWT)
			if status == types.TenantStatusActive {
				require.NoError(t, validateErr)
			} else {
				require.ErrorIs(t, validateErr, ErrTenantNotActive)
			}
		})
	}
}

func TestLogoutRevokesAllUserTokens(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)

	expiredAccess := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "access",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	if err := svc.Logout(ctx, expiredAccess); err != nil {
		t.Fatalf("Logout(expired access token) err = %v", err)
	}
	if len(tokenRepo.revokedUserIDs) != 1 || tokenRepo.revokedUserIDs[0] != "user-1" {
		t.Fatalf("RevokeTokensByUserID calls = %v, want [user-1]", tokenRepo.revokedUserIDs)
	}
}

func TestAdminResetPasswordHashesPasswordAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	if err := svc.AdminResetPassword(ctx, "user-1", "NewSecure9"); err != nil {
		t.Fatalf("AdminResetPassword() err = %v", err)
	}
	if repo.updateCalls != 1 {
		t.Fatalf("UpdateUser calls = %d, want 1", repo.updateCalls)
	}
	user := repo.users["user-1"]
	if user.PasswordHash == "NewSecure9" || user.PasswordHash == "" {
		t.Fatalf("password was not stored as a hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("NewSecure9")); err != nil {
		t.Fatalf("stored hash does not match new password: %v", err)
	}
	if len(tokenRepo.revokedUserIDs) != 1 || tokenRepo.revokedUserIDs[0] != "user-1" {
		t.Fatalf("RevokeTokensByUserID calls = %v, want [user-1]", tokenRepo.revokedUserIDs)
	}
}

func TestAdminResetPasswordRejectsWeakPasswordBeforeWrite(t *testing.T) {
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	err := svc.AdminResetPassword(context.Background(), "user-1", "password")
	if !errors.Is(err, ErrPasswordPolicy) {
		t.Fatalf("AdminResetPassword() err = %v, want ErrPasswordPolicy", err)
	}
	if repo.updateCalls != 0 || len(tokenRepo.revokedUserIDs) != 0 {
		t.Fatalf("weak password caused side effects: updates=%d revocations=%v",
			repo.updateCalls, tokenRepo.revokedUserIDs)
	}
}

func TestChangePasswordRequiresPolicyAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	hashed, err := bcrypt.GenerateFromPassword([]byte("OldSecure9"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	repo.users["user-1"].PasswordHash = string(hashed)

	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "weak"); !errors.Is(err, ErrPasswordPolicy) {
		t.Fatalf("ChangePassword(weak) err = %v, want ErrPasswordPolicy", err)
	}
	if repo.updateCalls != 0 || len(tokenRepo.revokedUserIDs) != 0 {
		t.Fatalf("weak password caused side effects: updates=%d revocations=%v",
			repo.updateCalls, tokenRepo.revokedUserIDs)
	}

	if err := svc.ChangePassword(ctx, "user-1", "wrong-pass", "NewSecure9"); !errors.Is(err, ErrInvalidOldPassword) {
		t.Fatalf("ChangePassword(wrong old) err = %v, want ErrInvalidOldPassword", err)
	}

	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "NewSecure9"); err != nil {
		t.Fatalf("ChangePassword() err = %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.users["user-1"].PasswordHash), []byte("NewSecure9")); err != nil {
		t.Fatalf("stored hash does not match new password: %v", err)
	}
	if len(tokenRepo.revokedUserIDs) != 1 || tokenRepo.revokedUserIDs[0] != "user-1" {
		t.Fatalf("revoked users = %v, want [user-1]", tokenRepo.revokedUserIDs)
	}
}

func TestChangePasswordRejectsSamePassword(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	repo := svc.userRepo.(*stubUserRepoForAuth)

	hashed, err := bcrypt.GenerateFromPassword([]byte("OldSecure9"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	repo.users["user-1"].PasswordHash = string(hashed)

	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "OldSecure9"); !errors.Is(err, ErrSamePassword) {
		t.Fatalf("ChangePassword(same) err = %v, want ErrSamePassword", err)
	}
	if repo.updateCalls != 0 || len(tokenRepo.revokedUserIDs) != 0 {
		t.Fatalf("same password caused side effects: updates=%d revocations=%v", repo.updateCalls, tokenRepo.revokedUserIDs)
	}
}

func TestChangePasswordHonoursRuntimeComplexPolicy(t *testing.T) {
	ctx := context.Background()
	tokenRepo := &stubAuthTokenRepo{tokens: map[string]*types.AuthToken{}}
	svc := newAuthTestUserService(tokenRepo)
	svc.systemSettingSvc = &stubComplexPasswordSettings{enabled: true}
	repo := svc.userRepo.(*stubUserRepoForAuth)

	hashed, err := bcrypt.GenerateFromPassword([]byte("OldSecure9"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	repo.users["user-1"].PasswordHash = string(hashed)

	if err := svc.ChangePassword(ctx, "user-1", "wrong-pass", "weak"); !errors.Is(err, ErrInvalidOldPassword) {
		t.Fatalf("ChangePassword(wrong old, weak new) err = %v, want ErrInvalidOldPassword", err)
	}
	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "NewSecure9"); !errors.Is(err, ErrComplexPasswordPolicy) {
		t.Fatalf("ChangePassword(simple new) err = %v, want ErrComplexPasswordPolicy", err)
	}
	if repo.updateCalls != 0 || len(tokenRepo.revokedUserIDs) != 0 {
		t.Fatalf("complex-policy reject caused side effects: updates=%d revocations=%v",
			repo.updateCalls, tokenRepo.revokedUserIDs)
	}
	if err := svc.ChangePassword(ctx, "user-1", "OldSecure9", "NewSecure9!"); err != nil {
		t.Fatalf("ChangePassword(complex new) err = %v", err)
	}
}

func TestUserIDFromSignedTokenAcceptsExpiredToken(t *testing.T) {
	expired := signTestJWT(jwt.MapClaims{
		"user_id": "user-1",
		"type":    "access",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})

	userID, err := userIDFromSignedToken(expired)
	if err != nil {
		t.Fatalf("userIDFromSignedToken(expired) err = %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("userIDFromSignedToken(expired) = %q, want user-1", userID)
	}
}
