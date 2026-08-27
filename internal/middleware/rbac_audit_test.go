package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type stubDenyAudit struct {
	interfaces.AuditLogService
	mu    sync.Mutex
	calls int
}

func (s *stubDenyAudit) LogDenied(
	context.Context, *gin.Context, uint64, string, string, types.TenantRole,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return nil
}

func auditableHarness(role types.TenantRole, audit interfaces.AuditLogService, mw gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuditServiceProvider(audit))
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, role)
		ctx = context.WithValue(ctx, types.UserIDContextKey, "u1")
		ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(7))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/protected", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	return w
}

func TestRequireRole_RejectFiresAuditHook(t *testing.T) {
	audit := &stubDenyAudit{}
	w := auditableHarness(types.TenantRoleContributor, audit, RequireRole(types.TenantRoleAdmin, cfgRBAC(true)))
	if w.Code != http.StatusForbidden || audit.calls != 1 {
		t.Fatalf("status=%d audit calls=%d, want 403 and one call", w.Code, audit.calls)
	}
}

func TestRequireRole_DormantModeDoesNotFireAuditHook(t *testing.T) {
	audit := &stubDenyAudit{}
	w := auditableHarness(types.TenantRoleViewer, audit, RequireRole(types.TenantRoleOwner, cfgRBAC(false)))
	if w.Code != http.StatusOK || audit.calls != 0 {
		t.Fatalf("status=%d audit calls=%d, want 200 and no call", w.Code, audit.calls)
	}
}
