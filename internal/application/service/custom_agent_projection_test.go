package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestAgentViewHidesPlatformBindingsFromWorkspace(t *testing.T) {
	agent := &types.CustomAgent{Config: types.CustomAgentConfig{
		ModelID:                       "chat",
		RerankModelID:                 "rerank",
		Temperature:                   0.7,
		MaxCompletionTokens:           4096,
		MaxIterations:                 8,
		AllowedTools:                  []string{"wiki_search"},
		MCPSelectionMode:              "selected",
		MCPServices:                   []string{"mcp-secret"},
		SkillsSelectionMode:           "selected",
		SelectedSkills:                []string{"skill-secret"},
		VLMModelID:                    "vlm",
		ASRModelID:                    "asr",
		WebSearchProviderID:           "search",
		ImageStorageProvider:          "storage",
		LLMCallTimeout:                90,
		ImageUploadEnabled:            true,
		AudioUploadEnabled:            true,
		AttachmentImageUnderstanding:  true,
		AttachmentOCRMaxPages:         12,
		AttachmentParseWaitTimeoutSec: 60,
		ChatParserEngineRules:         []types.ParserEngineRule{{Engine: "docreader"}},
		WebSearchEnabled:              true,
		EmbeddingTopK:                 20,
		RerankThreshold:               0.5,
	}}

	view := AgentView(context.Background(), agent)
	assert.Empty(t, view.Config.ModelID)
	assert.Empty(t, view.Config.RerankModelID)
	assert.Zero(t, view.Config.Temperature)
	assert.Zero(t, view.Config.MaxCompletionTokens)
	assert.Zero(t, view.Config.MaxIterations)
	assert.Empty(t, view.Config.AllowedTools)
	assert.Empty(t, view.Config.MCPSelectionMode)
	assert.Empty(t, view.Config.MCPServices)
	assert.Empty(t, view.Config.SkillsSelectionMode)
	assert.Empty(t, view.Config.SelectedSkills)
	assert.Empty(t, view.Config.VLMModelID)
	assert.Empty(t, view.Config.ASRModelID)
	assert.Empty(t, view.Config.WebSearchProviderID)
	assert.Empty(t, view.Config.ImageStorageProvider)
	assert.Zero(t, view.Config.LLMCallTimeout)
	assert.True(t, view.Config.ImageUploadEnabled)
	assert.False(t, view.Config.AudioUploadEnabled)
	assert.False(t, view.Config.AttachmentImageUnderstanding)
	assert.Zero(t, view.Config.AttachmentOCRMaxPages)
	assert.Zero(t, view.Config.AttachmentParseWaitTimeoutSec)
	assert.Empty(t, view.Config.ChatParserEngineRules)
	assert.True(t, view.Config.WebSearchEnabled)
	assert.Zero(t, view.Config.EmbeddingTopK)
	assert.Zero(t, view.Config.RerankThreshold)
	assert.Equal(t, "chat", agent.Config.ModelID)
}

func TestAgentViewKeepsPlatformBindingsForSystemAdmin(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.SystemAdminContextKey, true)
	agent := &types.CustomAgent{Config: types.CustomAgentConfig{ModelID: "chat"}}

	assert.Same(t, agent, AgentView(ctx, agent))
}

func TestAgentViewShowsOnlyScenarioControlsToEnterpriseAdmin(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleAdmin)
	agent := &types.CustomAgent{Config: types.CustomAgentConfig{
		ModelID:          "platform-model",
		AllowedTools:     []string{"knowledge_search"},
		MCPSelectionMode: "selected",
		MCPServices:      []string{"mcp-1"},
		WebSearchEnabled: true,
		LLMCallTimeout:   90,
	}}

	view := AgentView(ctx, agent)
	assert.Equal(t, []string{"knowledge_search"}, view.Config.AllowedTools)
	assert.Equal(t, "selected", view.Config.MCPSelectionMode)
	assert.Equal(t, []string{"mcp-1"}, view.Config.MCPServices)
	assert.True(t, view.Config.WebSearchEnabled)
	assert.Empty(t, view.Config.ModelID)
	assert.Zero(t, view.Config.LLMCallTimeout)
}

func TestWorkspaceAgentUpdateChangesBusinessFieldsOnly(t *testing.T) {
	current := types.CustomAgentConfig{
		SystemPrompt: "old", ModelID: "platform-model", Temperature: 0.4,
		AllowedTools: []string{"platform-tool"},
	}
	next := types.CustomAgentConfig{
		SystemPrompt: "new", ModelID: "workspace-model", Temperature: 1,
		AllowedTools: []string{"workspace-tool"},
	}

	preserveAgentPlatformBindings(&next, current)

	assert.Equal(t, "new", next.SystemPrompt)
	assert.Equal(t, "platform-model", next.ModelID)
	assert.Equal(t, 0.4, next.Temperature)
	assert.Equal(t, []string{"workspace-tool"}, next.AllowedTools)
}
