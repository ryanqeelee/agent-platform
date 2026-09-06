package handler

import (
	"bytes"
	"context"
	"encoding/json"
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

// stubRegisterUserService is a UserService whose ONLY useful method is
// Register; every other call panics. Using an interface embedding plus a
// targeted override keeps the test focused on the Register handler's
// branching logic without dragging in the entire user service surface.
type stubRegisterUserService struct {
	interfaces.UserService
	register func(ctx context.Context, req *types.RegisterRequest) (*types.User, error)
}

func (s *stubRegisterUserService) Register(ctx context.Context, req *types.RegisterRequest) (*types.User, error) {
	return s.register(ctx, req)
}

// errorCapture mirrors gin's default ErrorHandler behaviour for tests:
// when a handler calls c.Error(), we surface it as an HTTP response so the
// recorder reflects the real client-visible status. The production
// middleware does the same thing in middleware/error_handler.go.
func errorCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err
		if appErr, ok := err.(*apperrors.AppError); ok {
			c.JSON(appErr.HTTPCode, gin.H{"error": appErr.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func newRegisterTestRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	r.POST("/auth/register", h.Register)
	return r
}

func doRegister(t *testing.T, r *gin.Engine, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// validRegisterBody returns a payload that passes parameter validation, so
// each test is exercising the gate logic and not the body parser.
func validRegisterBody() map[string]string {
	return map[string]string{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "supersecret1",
	}
}

func TestRegister_InviteOnlyRejects(t *testing.T) {
	// PR 3 (#1303): when auth.registration_mode=invite_only, Register
	// must respond 403 BEFORE touching the user service. The frontend
	// already hides the sign-up link via /auth/config; this is the
	// server-side enforcement for direct API hits.
	called := false
	us := &stubRegisterUserService{
		register: func(context.Context, *types.RegisterRequest) (*types.User, error) {
			called = true
			return &types.User{ID: "u1"}, nil
		},
	}
	h := NewAuthHandler(&config.Config{
		Auth: &config.AuthConfig{RegistrationMode: config.AuthRegistrationModeInviteOnly},
	}, us, nil, nil, nil, nil)

	w := doRegister(t, newRegisterTestRouter(h), validRegisterBody())
	if w.Code != http.StatusForbidden {
		t.Fatalf("invite_only must return 403, got %d body=%s", w.Code, w.Body.String())
	}
	if called {
		t.Fatalf("UserService.Register must not be called when invite_only blocks the request")
	}
}

func TestRegister_SelfServeConfigCannotReopenRegistration(t *testing.T) {
	called := false
	us := &stubRegisterUserService{
		register: func(_ context.Context, _ *types.RegisterRequest) (*types.User, error) {
			called = true
			return &types.User{ID: "u1", Email: "alice@example.com"}, nil
		},
	}
	h := NewAuthHandler(&config.Config{
		Auth: &config.AuthConfig{RegistrationMode: config.AuthRegistrationModeSelfServe},
	}, us, nil, nil, nil, nil)

	w := doRegister(t, newRegisterTestRouter(h), validRegisterBody())
	if w.Code != http.StatusForbidden {
		t.Fatalf("product admission must remain invite-only, got %d body=%s", w.Code, w.Body.String())
	}
	if called {
		t.Fatalf("self_serve configuration must not reach UserService.Register")
	}
}

func TestGetAuthConfigExposesComplexPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{
		configInfo: &config.Config{Auth: &config.AuthConfig{
			RegistrationMode: config.AuthRegistrationModeSelfServe,
		}},
		systemSettingSvc: &tenantPolicySettingService{enabled: true},
	}
	r := gin.New()
	r.GET("/auth/config", h.GetAuthConfig)
	req := httptest.NewRequest(http.MethodGet, "/auth/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"complex_password_enabled":true`) {
		t.Fatalf("body=%s, want complex_password_enabled true", w.Body.String())
	}
}

func TestRegister_NilAuthConfigDoesNotPanic(t *testing.T) {
	called := false
	us := &stubRegisterUserService{
		register: func(_ context.Context, _ *types.RegisterRequest) (*types.User, error) {
			called = true
			return &types.User{ID: "u1", Email: "alice@example.com"}, nil
		},
	}
	h := NewAuthHandler(&config.Config{}, us, nil, nil, nil, nil)

	w := doRegister(t, newRegisterTestRouter(h), validRegisterBody())
	if w.Code != http.StatusForbidden {
		t.Fatalf("nil Auth config must stay invite-only, got %d body=%s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("nil Auth config reached UserService.Register")
	}
}

func TestRegister_InviteOnlyRejectsBeforeParsingCredentials(t *testing.T) {
	called := false
	us := &stubRegisterUserService{
		register: func(_ context.Context, _ *types.RegisterRequest) (*types.User, error) {
			called = true
			return &types.User{ID: "u1", Email: "alice@example.com"}, nil
		},
	}
	h := NewAuthHandler(&config.Config{}, us, nil, nil, nil, nil)
	w := doRegister(t, newRegisterTestRouter(h), map[string]string{"password": "abc\t123!\nX"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("invite-only admission must reject before parsing, got %d body=%s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("closed public registration reached UserService.Register")
	}
}
