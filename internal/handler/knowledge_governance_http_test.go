package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type governanceBatchKnowledgeService struct {
	interfaces.KnowledgeService
	rows []*types.Knowledge
}

func (s *governanceBatchKnowledgeService) GetKnowledgeBatchWithSharedAccess(context.Context, uint64, []string) ([]*types.Knowledge, error) {
	return s.rows, nil
}

func TestGetKnowledgeBatchRejectsMixedAuthorizedAndUnauthorizedIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "employee")
		c.Next()
	})
	h := &KnowledgeHandler{kgService: &governanceBatchKnowledgeService{rows: []*types.Knowledge{
		{ID: "allowed", TenantID: 1, KnowledgeBaseID: "kb-allowed"},
	}}}
	r.GET("/knowledge/batch", h.GetKnowledgeBatch)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/knowledge/batch?ids=allowed&ids=denied", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	if w.Body.String() == "" {
		t.Fatal("expected a stable error response")
	}
}

type governanceKBResponseService struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *governanceKBResponseService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func (s *governanceKBResponseService) FillKnowledgeBaseCounts(context.Context, *types.KnowledgeBase) error {
	return nil
}

func (s *governanceKBResponseService) TogglePinKnowledgeBase(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func newGovernanceKBResponseRouter(t *testing.T, role types.TenantRole, method string) *gin.Engine {
	t.Helper()
	storeID := "store-secret"
	kb := &types.KnowledgeBase{
		ID: "kb", TenantID: 1, EmbeddingModelID: "embed-secret",
		SummaryModelID: "summary-secret", StorageBackendID: &storeID, VectorStoreID: &storeID,
	}
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(1))
		c.Set(types.UserIDContextKey.String(), "employee")
		c.Set(middleware.KBAccessContextKey, &middleware.KBAccess{
			KnowledgeBase: kb, EffectiveTenantID: 1, Permission: types.OrgRoleAdmin,
		})
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, role)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &KnowledgeBaseHandler{service: &governanceKBResponseService{kb: kb}}
	if method == http.MethodGet {
		r.GET("/knowledge-bases/:id", h.GetKnowledgeBase)
	} else {
		r.PUT("/knowledge-bases/:id/pin", h.TogglePinKnowledgeBase)
	}
	return r
}

func TestKnowledgeBaseReadAndWriteResponsesRedactInfrastructureForKnowledgeAdministrator(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		path := "/knowledge-bases/kb"
		if method == http.MethodPut {
			path += "/pin"
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		newGovernanceKBResponseRouter(t, types.TenantRoleContributor, method).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", method, w.Code, w.Body.String())
		}
		for _, platformDetail := range []string{"embed-secret", "summary-secret", "store-secret"} {
			if contains := stringContains(w.Body.String(), platformDetail); contains {
				t.Fatalf("%s response leaked %q: %s", method, platformDetail, w.Body.String())
			}
		}
	}
}

func stringContains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
