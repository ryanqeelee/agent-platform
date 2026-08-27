package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func contentRoleContext(role types.TenantRole, tenantID uint64) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/knowledge", nil)
	c.Set(types.TenantIDContextKey.String(), tenantID)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), types.TenantRoleContextKey, role))
	return c
}

func TestRequireKBContentRoleUsesCurrentRoleNotCreator(t *testing.T) {
	h := &KnowledgeHandler{}
	cases := []struct {
		name    string
		role    types.TenantRole
		tenant  uint64
		kb      *types.KnowledgeBase
		wantErr bool
	}{
		{
			name: "own contributor may maintain another users knowledge base",
			role: types.TenantRoleContributor, tenant: 1,
			kb: &types.KnowledgeBase{ID: "own", TenantID: 1, CreatorID: "someone-else"},
		},
		{
			name: "own viewer is denied despite being creator",
			role: types.TenantRoleViewer, tenant: 1,
			kb: &types.KnowledgeBase{ID: "own", TenantID: 1, CreatorID: "u-viewer"}, wantErr: true,
		},
		{
			name: "foreign shared admin may maintain content",
			role: types.TenantRoleAdmin, tenant: 1,
			kb: &types.KnowledgeBase{ID: "foreign", TenantID: 2, CreatorID: "source-user"},
		},
		{
			name: "foreign shared contributor is denied",
			role: types.TenantRoleContributor, tenant: 1,
			kb: &types.KnowledgeBase{ID: "foreign", TenantID: 2, CreatorID: "source-user"}, wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := h.requireKBContentRole(contentRoleContext(tc.role, tc.tenant), tc.kb)
			if (err != nil) != tc.wantErr {
				t.Fatalf("requireKBContentRole error = %v, want error=%v", err, tc.wantErr)
			}
		})
	}
}

func TestRequireKBContentRoleKeepsAPIKeyOnRouteAuthorizedPath(t *testing.T) {
	h := &KnowledgeHandler{}
	c := contentRoleContext(types.TenantRoleViewer, 1)
	c.Request = c.Request.WithContext(types.WithTenantAPIKeyScope(c.Request.Context(), types.TenantAPIKeyScope{
		Capabilities:     types.StringArray{string(types.APIKeyCapabilityIngest)},
		KnowledgeBaseIDs: types.StringArray{"kb-1"},
	}))
	if err := h.requireKBContentRole(c, &types.KnowledgeBase{ID: "kb-1", TenantID: 1}); err != nil {
		t.Fatalf("API-key content route already authorized by APIKeyGate and allow-list was re-rejected: %v", err)
	}
}

func TestAuthorizedSharedKnowledgeContextPreservesExactBodyMutationProvenance(t *testing.T) {
	c := contentRoleContext(types.TenantRoleAdmin, 1)
	ctx := authorizedSharedKnowledgeContext(c, c.Request.Context(), "kb-source", 2)

	if tenantID, ok := types.TenantIDFromContext(ctx); !ok || tenantID != 2 {
		t.Fatalf("effective tenant = %d, %v; want source tenant 2", tenantID, ok)
	}
	if !types.HasAuthorizedSharedKnowledgeBase(ctx, 2, "kb-source") {
		t.Fatal("body mutation context must preserve exact shared KB provenance")
	}
	if types.HasAuthorizedSharedKnowledgeBase(ctx, 2, "kb-sibling") {
		t.Fatal("body mutation context must not authorize a sibling KB")
	}
}
