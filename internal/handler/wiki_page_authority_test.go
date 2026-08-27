package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type wikiAuthorityKBService struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s wikiAuthorityKBService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func TestValidateWikiKBUsesAuthorizedEffectiveTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &WikiPageHandler{kbService: wikiAuthorityKBService{kb: &types.KnowledgeBase{
		ID: "kb", TenantID: 2, IndexingStrategy: types.IndexingStrategy{WikiEnabled: true},
	}}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "kb_id", Value: "kb"}}
	c.Set(types.TenantIDContextKey.String(), uint64(1))
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(2))
	c.Request = httptest.NewRequest("GET", "/knowledgebase/kb/wiki/pages", nil).WithContext(ctx)

	_, tenantID, err := h.validateWikiKB(c)
	if err != nil || tenantID != 2 {
		t.Fatalf("tenant=%d err=%v, want authorized source tenant 2", tenantID, err)
	}
}
