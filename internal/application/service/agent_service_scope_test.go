package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestAgentHasKnowledgeScope_TagOnlySearchTargets(t *testing.T) {
	cfg := &types.AgentConfig{
		SearchTargets: types.SearchTargets{
			{
				Type:            types.SearchTargetTypeKnowledgeBase,
				KnowledgeBaseID: "kb-1",
				TagIDs:          []string{"tag-a"},
			},
		},
	}
	assert.True(t, agentHasKnowledgeScope(cfg))
}

func TestAgentHasKnowledgeScope_Empty(t *testing.T) {
	assert.False(t, agentHasKnowledgeScope(&types.AgentConfig{}))
	assert.False(t, agentHasKnowledgeScope(nil))
}

func TestKnowledgeBaseScopesForPrompt_FromSearchTargets(t *testing.T) {
	cfg := &types.AgentConfig{
		SearchTargets: types.SearchTargets{
			{KnowledgeBaseID: "kb-1", TagIDs: []string{"tag-a"}},
			{KnowledgeBaseID: "kb-1", TagIDs: []string{"tag-b"}},
			{KnowledgeBaseID: "kb-2", TagIDs: []string{"tag-c"}},
		},
	}
	ids, _ := knowledgeBaseScopesForPrompt(cfg)
	assert.Equal(t, []string{"kb-1", "kb-2"}, ids)
}

func TestKnowledgeBaseScopesForPrompt_CarriesSharedKBSourceTenant(t *testing.T) {
	cfg := &types.AgentConfig{
		SearchTargets: types.SearchTargets{
			{KnowledgeBaseID: "shared-kb", TenantID: 42},
			{KnowledgeBaseID: "own-kb", TenantID: 0},
		},
	}
	ids, tenantMap := knowledgeBaseScopesForPrompt(cfg)
	assert.Equal(t, []string{"shared-kb", "own-kb"}, ids)
	assert.Equal(t, uint64(42), tenantMap["shared-kb"])
	assert.Zero(t, tenantMap["own-kb"])
}

// KnowledgeBases carries shared KB IDs too (KBSelectionMode="all" and @mentions
// both write into it), so the source tenant must still come from SearchTargets.
func TestKnowledgeBaseScopesForPrompt_ExplicitKnowledgeBasesKeepSourceTenant(t *testing.T) {
	cfg := &types.AgentConfig{
		KnowledgeBases: []string{"own-kb", "shared-kb"},
		SearchTargets: types.SearchTargets{
			{KnowledgeBaseID: "own-kb", TenantID: 7},
			{KnowledgeBaseID: "shared-kb", TenantID: 42},
		},
	}
	ids, tenantMap := knowledgeBaseScopesForPrompt(cfg)
	assert.Equal(t, []string{"own-kb", "shared-kb"}, ids)
	assert.Equal(t, uint64(42), tenantMap["shared-kb"])
}

func TestKnowledgeBaseScopesForPrompt_NilConfig(t *testing.T) {
	ids, tenantMap := knowledgeBaseScopesForPrompt(nil)
	assert.Nil(t, ids)
	assert.Empty(t, tenantMap)
}

func TestRegisteredKnowledgeWriteAuthorityKeepsScopedAPIKeyWikiWrites(t *testing.T) {
	viewerCtx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleViewer)
	contributorCtx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleAdmin)
	cases := []struct {
		name      string
		ctx       context.Context
		wantWrite bool
	}{
		{"human viewer", viewerCtx, false},
		{"human administrator", contributorCtx, true},
		{"retrieve-only key", types.WithTenantAPIKeyScope(viewerCtx, types.TenantAPIKeyScope{Capabilities: types.StringArray{string(types.APIKeyCapabilityRetrieve)}}), false},
		{"full key", types.WithTenantAPIKeyScope(viewerCtx, types.TenantAPIKeyScope{FullAccess: true}), true},
		{"manage knowledge bases key", types.WithTenantAPIKeyScope(viewerCtx, types.TenantAPIKeyScope{Capabilities: types.StringArray{string(types.APIKeyCapabilityManageKnowledgeBases)}}), true},
		{"ingest key", types.WithTenantAPIKeyScope(viewerCtx, types.TenantAPIKeyScope{Capabilities: types.StringArray{string(types.APIKeyCapabilityIngest)}}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowed := []string{tools.ToolWikiReadPage, tools.ToolWikiWritePage}
			if !hasRegisteredKnowledgeWriteAuthority(tc.ctx) {
				allowed = filterWikiWriteTools(allowed)
			}
			assert.Equal(t, tc.wantWrite, hasRegisteredKnowledgeWriteAuthority(tc.ctx))
			assert.Equal(t, tc.wantWrite, containsString(allowed, tools.ToolWikiWritePage))
			assert.True(t, containsString(allowed, tools.ToolWikiReadPage))
		})
	}
}

func TestKnowledgeScopedToolClassifierCoversDefaultDataTools(t *testing.T) {
	for _, name := range []string{tools.ToolDataAnalysis, tools.ToolDataSchema} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, isKnowledgeScopedTool(name), "default tool with SearchTargets must receive live target authorization")
		})
	}
}
