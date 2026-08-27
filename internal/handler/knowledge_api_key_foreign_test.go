package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type apiKeyForeignKBServiceStub struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s apiKeyForeignKBServiceStub) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func (s apiKeyForeignKBServiceStub) GetKnowledgeBaseByIDOnly(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

type apiKeyForeignKnowledgeServiceStub struct {
	interfaces.KnowledgeService
	knowledge *types.Knowledge
}

func (s apiKeyForeignKnowledgeServiceStub) GetKnowledgeByIDOnly(context.Context, string) (*types.Knowledge, error) {
	return s.knowledge, nil
}

func apiKeyForeignHandlerContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set(types.TenantIDContextKey.String(), uint64(1))
	ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(1))
	ctx = types.WithTenantAPIKeyScope(ctx, types.TenantAPIKeyScope{FullAccess: true})
	c.Request = c.Request.WithContext(ctx)
	return c
}

func TestDirectKnowledgeBaseResolverRejectsForeignKBForAPIKeyBeforeShares(t *testing.T) {
	h := &KnowledgeHandler{kbService: apiKeyForeignKBServiceStub{
		kb: &types.KnowledgeBase{ID: "foreign", TenantID: 2},
	}}
	if _, _, _, _, err := h.validateKnowledgeBaseAccessWithKBID(apiKeyForeignHandlerContext(), "foreign"); err == nil {
		t.Fatal("API key must not inherit a foreign KB share")
	}
}

func TestDirectKnowledgeResolverRejectsForeignKBForAPIKeyBeforeShares(t *testing.T) {
	h := &KnowledgeHandler{
		kgService: apiKeyForeignKnowledgeServiceStub{
			knowledge: &types.Knowledge{ID: "doc", TenantID: 2, KnowledgeBaseID: "foreign"},
		},
		kbService: apiKeyForeignKBServiceStub{
			kb: &types.KnowledgeBase{ID: "foreign", TenantID: 2},
		},
	}
	if _, _, err := h.resolveKnowledgeAndValidateKBAccess(apiKeyForeignHandlerContext(), "doc", types.OrgRoleViewer); err == nil {
		t.Fatal("API key must not inherit a foreign document share")
	}
}
