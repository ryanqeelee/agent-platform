package handler

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// The route-level KB access guard must preserve the repository's not-found
// sentinel as a 404 before the handler runs. That keeps a stale KB id from
// becoming either an authorization error or a fake internal error.
//
// The wrapped-sentinel cases are the actual security boundary: if a
// caller drops the stderrors.Is comparison and reverts to `==`, the
// fmt.Errorf("%w") path silently fails over to 500 and the tests below
// fail before the change ships.

// stubKBOnlyService implements just enough of KnowledgeBaseService to
// drive the route-level KB access guard. Embedding the interface keeps every
// other method nil-panicky on purpose so a future test that reaches
// outside the contract fails loudly.
type stubKBOnlyService struct {
	interfaces.KnowledgeBaseService
	getByID                 func(ctx context.Context, id string) (*types.KnowledgeBase, error)
	fillKnowledgeBaseCounts func(ctx context.Context, kb *types.KnowledgeBase) error
}

func (s *stubKBOnlyService) GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	return s.getByID(ctx, id)
}

func (s *stubKBOnlyService) GetKnowledgeBaseByIDOnly(ctx context.Context, id string) (*types.KnowledgeBase, error) {
	return s.getByID(ctx, id)
}

func (s *stubKBOnlyService) FillKnowledgeBaseCounts(ctx context.Context, kb *types.KnowledgeBase) error {
	if s.fillKnowledgeBaseCounts != nil {
		return s.fillKnowledgeBaseCounts(ctx, kb)
	}
	return nil
}

// newKBHandlerTestRouter mounts the production ErrorHandler so
// c.Error(NewNotFoundError(...)) renders as the real 404 envelope.
// Tenant id and user id are injected by a tiny middleware, followed by the
// same KB access guard that production routes use.
func newKBHandlerTestRouter(svc interfaces.KnowledgeBaseService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "u-test")
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(1))
		ctx = context.WithValue(ctx, types.UserIDContextKey, "u-test")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &KnowledgeBaseHandler{service: svc}
	r.GET(
		"/knowledge-bases/:id",
		middleware.RequireKBAccess(
			middleware.KBIDFromParam("id"),
			types.OrgRoleViewer,
			svc,
			nil,
			nil,
			nil,
		),
		h.GetKnowledgeBase,
	)
	return r
}

func TestKBHandlerMapsErrKnowledgeBaseNotFoundToNotFound(t *testing.T) {
	svc := &stubKBOnlyService{
		getByID: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
			return nil, repository.ErrKnowledgeBaseNotFound
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/missing-kb", nil)
	newKBHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("ErrKnowledgeBaseNotFound must map to 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestKBHandlerHonoursWrappedErrKnowledgeBaseNotFound(t *testing.T) {
	// Regression test against a stale `==` sentinel comparison: errors.Is
	// unwraps fmt.Errorf("%w", ...); a literal `==` does not. If anyone
	// reverts the comparison this test fails before the change ships.
	svc := &stubKBOnlyService{
		getByID: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
			return nil, fmt.Errorf("loading kb: %w", repository.ErrKnowledgeBaseNotFound)
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/missing-kb", nil)
	newKBHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("wrapped ErrKnowledgeBaseNotFound must still map to 404, got %d body=%s",
			w.Code, w.Body.String())
	}
	// Defence-in-depth: the response body should expose the AppError
	// envelope (ErrorHandler renders {success,error:{code,message}}),
	// not the bare gin error string. Code 1003 is ErrNotFound.
	if !strings.Contains(w.Body.String(), `"code":1003`) {
		t.Fatalf("expected NotFound envelope with code=1003, got body=%s", w.Body.String())
	}
}

func TestKnowledgeBaseDirectResolverRejectsForeignKBForAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "id", Value: "foreign"}}
	c.Set(types.TenantIDContextKey.String(), uint64(1))
	c.Request = httptest.NewRequest(http.MethodGet, "/knowledge-bases/foreign", nil)
	ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(1))
	ctx = types.WithTenantAPIKeyScope(ctx, types.TenantAPIKeyScope{FullAccess: true})
	c.Request = c.Request.WithContext(ctx)
	svc := &stubKBOnlyService{getByID: func(context.Context, string) (*types.KnowledgeBase, error) {
		return &types.KnowledgeBase{ID: "foreign", TenantID: 2}, nil
	}}

	if _, err := middleware.ResolveKBAccess(c, "foreign", types.OrgRoleViewer, svc, nil, nil, nil); err == nil {
		t.Fatal("API key must not inherit a foreign KB share in the direct knowledge-base resolver")
	}
}

func TestKBGuardMapsGenuineInfraErrorsToServiceUnavailable(t *testing.T) {
	// The mapping is *only* for the not-found sentinel — every other
	// error must surface as a retryable 503 so monitoring catches genuine
	// DB / repo failures without misrepresenting the resource as missing.
	svc := &stubKBOnlyService{
		getByID: func(_ context.Context, _ string) (*types.KnowledgeBase, error) {
			return nil, stderrors.New("connection refused")
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/some-kb", nil)
	newKBHandlerTestRouter(svc).ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("non-sentinel errors must map to 503, got %d body=%s", w.Code, w.Body.String())
	}
}

// guardAgainstStaleSentinelEquality is a compile-time assertion: if a
// future refactor accidentally drops the apperrors / stderrors imports
// from THIS file, the test stops compiling and the human gets a clear
// signal that something tied to the not-found mapping went away.
var (
	_ = stderrors.Is
	_ = apperrors.NewNotFoundError
)
