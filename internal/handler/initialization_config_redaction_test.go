package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildConfigResponse_ViewerOmitsModelInfrastructure(t *testing.T) {
	h := &InitializationHandler{}
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleViewer)
	models := []*types.Model{{
		Type: types.ModelTypeKnowledgeQA,
		Name: "custom-llm",
		Parameters: types.ModelParameters{
			BaseURL: "https://tenant-private.example.com",
			APIKey:  "sk-secret-do-not-leak",
		},
	}}
	kb := &types.KnowledgeBase{
		ExtractConfig: &types.ExtractConfig{Enabled: true, CustomInstructions: "platform graph prompt"},
		QuestionGenerationConfig: &types.QuestionGenerationConfig{
			Enabled: true, CustomInstructions: "platform question prompt",
		},
	}

	config := h.buildConfigResponse(ctx, models, kb, false)
	llm, ok := config["llm"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, llm["baseUrl"])

	body, err := json.Marshal(config)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "tenant-private.example.com")
	assert.NotContains(t, string(body), "sk-secret-do-not-leak")
	assert.NotContains(t, string(body), "custom-llm")
	assert.NotContains(t, config, "nodeExtract")
	assert.NotContains(t, config, "questionGeneration")
}

func TestBuildConfigResponse_EnterpriseAdminOmitsModelInfrastructure(t *testing.T) {
	h := &InitializationHandler{}
	ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, types.TenantRoleAdmin)
	models := []*types.Model{{
		Type: types.ModelTypeKnowledgeQA,
		Name: "custom-llm",
		Parameters: types.ModelParameters{
			BaseURL: "https://tenant-private.example.com",
			APIKey:  "sk-secret-do-not-leak",
		},
	}}
	kb := &types.KnowledgeBase{}

	config := h.buildConfigResponse(ctx, models, kb, false)
	llm, ok := config["llm"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, llm["configured"])

	body, err := json.Marshal(config)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "tenant-private.example.com")
	assert.NotContains(t, string(body), "sk-secret-do-not-leak")
	assert.NotContains(t, string(body), "custom-llm")
}

func TestBuildConfigResponse_SystemAdminKeepsModelInfrastructure(t *testing.T) {
	h := &InitializationHandler{}
	ctx := context.WithValue(context.Background(), types.SystemAdminContextKey, true)
	models := []*types.Model{{
		Type: types.ModelTypeKnowledgeQA,
		Name: "custom-llm",
		Parameters: types.ModelParameters{
			BaseURL: "https://platform.example.com",
		},
	}}

	config := h.buildConfigResponse(ctx, models, &types.KnowledgeBase{}, false)
	llm := config["llm"].(map[string]interface{})
	assert.Equal(t, "custom-llm", llm["modelName"])
	assert.Equal(t, "https://platform.example.com", llm["baseUrl"])
}
