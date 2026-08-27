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

func TestKnowledgeBaseIDsForPrompt_FromSearchTargets(t *testing.T) {
	cfg := &types.AgentConfig{
		SearchTargets: types.SearchTargets{
			{KnowledgeBaseID: "kb-1", TagIDs: []string{"tag-a"}},
			{KnowledgeBaseID: "kb-1", TagIDs: []string{"tag-b"}},
			{KnowledgeBaseID: "kb-2", TagIDs: []string{"tag-c"}},
		},
	}
	assert.Equal(t, []string{"kb-1", "kb-2"}, knowledgeBaseIDsForPrompt(cfg))
}

func TestRegisteredKnowledgeWriteAuthorityKeepsScopedAPIKeyWikiWrites(t *testing.T) {
	viewerCtx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleViewer)
	contributorCtx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleContributor)
	cases := []struct {
		name      string
		ctx       context.Context
		wantWrite bool
	}{
		{"human viewer", viewerCtx, false},
		{"human contributor", contributorCtx, true},
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
