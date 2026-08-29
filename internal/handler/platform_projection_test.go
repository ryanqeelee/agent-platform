package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetSystemInfoHidesRuntimeDetailsFromWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/system/info", nil)

	(&SystemHandler{}).GetSystemInfo(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, map[string]interface{}{"edition": Edition}, body.Data)
}

type capabilityProjectionResolver struct {
	interfaces.AICapabilityPlanResolver
}

func (capabilityProjectionResolver) Resolve(context.Context, uint64) (*types.AICapabilityPlanResolution, error) {
	return &types.AICapabilityPlanResolution{
		ContractVersion: "AICapabilityPlanV1",
		PlanVersionID:   "hidden-plan",
		Source:          "platform_default",
		Enterprise: types.EnterpriseAICapabilityProjection{
			ServiceLevel: "standard",
			Status:       "active",
			Health:       types.EnterpriseAIHealth{Status: "unknown"},
		},
	}, nil
}

func TestEnterpriseAICapabilityProjectionHidesPlanIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(types.TenantIDContextKey.String(), uint64(7))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/7/ai-capability-plan", nil)

	NewAICapabilityPlanHandler(capabilityProjectionResolver{}).GetEnterpriseProjection(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"success":true,
		"data":{
			"service_level":"standard",
			"status":"active",
			"usage":null,
			"quota":null,
			"health":{"status":"unknown"}
		}
	}`, recorder.Body.String())
}

type workspaceWebSearchRepo struct {
	interfaces.WebSearchProviderRepository
}

func (workspaceWebSearchRepo) List(context.Context, uint64) ([]*types.WebSearchProviderEntity, error) {
	return []*types.WebSearchProviderEntity{{
		ID: "provider-secret", Name: "provider-name", IsDefault: true,
	}}, nil
}

func TestListWebSearchProvidersReturnsOnlyReadinessToWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(types.TenantIDContextKey.String(), uint64(7))
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/web-search-providers", nil)

	(&WebSearchProviderHandler{repo: workspaceWebSearchRepo{}}).ListProviders(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"success":true,"data":[{"is_default":true}]}`, recorder.Body.String())
}

type workspaceKBUpdateService struct {
	interfaces.KnowledgeBaseService
	config  *types.KnowledgeBaseConfig
	created *types.KnowledgeBase
}

func (s *workspaceKBUpdateService) CreateKnowledgeBase(_ context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	s.created = kb
	kb.ID = "kb-new"
	kb.TenantID = 7
	return kb, nil
}

func (s *workspaceKBUpdateService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return &types.KnowledgeBase{ID: "kb", TenantID: 7, Name: "before"}, nil
}

func (s *workspaceKBUpdateService) UpdateKnowledgeBase(
	_ context.Context, id, name, description string, config *types.KnowledgeBaseConfig,
) (*types.KnowledgeBase, error) {
	s.config = config
	return &types.KnowledgeBase{ID: id, TenantID: 7, Name: name, Description: description}, nil
}

func TestWorkspaceKnowledgeBaseUpdateIgnoresPlatformConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &workspaceKBUpdateService{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(types.TenantIDContextKey.String(), uint64(7))
	c.Params = gin.Params{{Key: "id", Value: "kb"}}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	c.Request = httptest.NewRequest(http.MethodPut, "/knowledge-bases/kb", strings.NewReader(`{
		"name":"after","description":"business",
		"config":{
			"faq_config":{"index_mode":"question_only","question_index_mode":"separate"},
			"chunking_config":{"chunk_size":9999},
			"wiki_config":{"synthesis_model_id":"model-secret"},
			"indexing_strategy":{"vector_enabled":false}
		}
	}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.KBAccessContextKey, &middleware.KBAccess{
		KnowledgeBase:     &types.KnowledgeBase{ID: "kb", TenantID: 7},
		EffectiveTenantID: 7,
		Permission:        types.OrgRoleAdmin,
	})

	(&KnowledgeBaseHandler{service: service}).UpdateKnowledgeBase(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, service.config)
	require.NotNil(t, service.config.FAQConfig)
	require.Zero(t, service.config.ChunkingConfig.ChunkSize)
	require.Nil(t, service.config.WikiConfig)
	require.Nil(t, service.config.IndexingStrategy)
}

func TestWorkspaceKnowledgeBaseCreateIgnoresPlatformBindings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &workspaceKBUpdateService{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(types.TenantIDContextKey.String(), uint64(7))
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	c.Request = httptest.NewRequest(http.MethodPost, "/knowledge-bases", strings.NewReader(`{
		"name":"retail","description":"business","type":"document",
		"embedding_model_id":"model-secret","summary_model_id":"summary-secret",
		"vector_store_id":"store-secret","storage_backend_id":"backend-secret",
		"image_processing_config":{"model_id":"image-secret"},
		"chunking_config":{"chunk_size":512,"parser_engine_rules":[{"file_types":["pdf"],"engine":"secret-parser"}]},
		"extract_config":{"enabled":true,"text":"platform-secret"},
		"wiki_config":{"synthesis_model_id":"wiki-secret","max_pages_per_ingest":8,"ingest_map_parallel":99},
		"indexing_strategy":{"vector_enabled":false,"keyword_enabled":false,"graph_enabled":true}
	}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")

	(&KnowledgeBaseHandler{service: service}).CreateKnowledgeBase(c)

	require.Equal(t, http.StatusCreated, recorder.Code)
	require.NotNil(t, service.created)
	require.Equal(t, "retail", service.created.Name)
	require.Equal(t, 512, service.created.ChunkingConfig.ChunkSize)
	require.Empty(t, service.created.ChunkingConfig.ParserEngineRules)
	require.Nil(t, service.created.ExtractConfig)
	require.True(t, service.created.IndexingStrategy.IsZero())
	require.Empty(t, service.created.EmbeddingModelID)
	require.Empty(t, service.created.ImageProcessingConfig.ModelID)
	require.Nil(t, service.created.VectorStoreID)
	require.Nil(t, service.created.StorageBackendID)
	require.NotNil(t, service.created.WikiConfig)
	require.Empty(t, service.created.WikiConfig.SynthesisModelID)
	require.Zero(t, service.created.WikiConfig.IngestMapParallel)
	require.Equal(t, 8, service.created.WikiConfig.MaxPagesPerIngest)
}
