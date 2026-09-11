package chatpipeline

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type fixedScoreReranker struct {
	results []rerank.RankResult
}

func (r *fixedScoreReranker) Rerank(
	context.Context,
	string,
	[]string,
) ([]rerank.RankResult, error) {
	return r.results, nil
}

func (r *fixedScoreReranker) GetModelName() string { return "fixed-score" }
func (r *fixedScoreReranker) GetModelID() string   { return "fixed-score" }

type rerankModelService struct {
	interfaces.ModelService
	model     rerank.Reranker
	models    []*types.Model
	getCalls  int
	listCalls int
}

func (s *rerankModelService) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	s.getCalls++
	return s.model, nil
}

func (s *rerankModelService) ListModels(context.Context) ([]*types.Model, error) {
	s.listCalls++
	return s.models, nil
}

func TestSuccessfulRerankUsesModelRelevanceAcrossMixedSources(t *testing.T) {
	model := &fixedScoreReranker{results: []rerank.RankResult{
		{Index: 1, RelevanceScore: 0.87},
		{Index: 0, RelevanceScore: 0.10},
	}}
	manager := NewEventManager()
	NewPluginRerank(manager, &rerankModelService{model: model})

	chatManage := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			RerankModelID:   "builtin-agent-rerank",
			RerankTopK:      1,
			RerankThreshold: 0,
		},
		PipelineState: types.PipelineState{
			RewriteQuery: "WeKnora v0.8.0 release notes",
			SearchResult: []*types.SearchResult{
				{
					ID:              "kb-low-relevance",
					Content:         "unrelated camera operating instructions",
					Score:           14,
					KnowledgeSource: "file",
				},
				{
					ID:              "web-high-relevance",
					Content:         "official WeKnora v0.8.0 release notes",
					Score:           0.20,
					KnowledgeSource: "web_search",
				},
			},
		},
	}

	require.Nil(t, manager.Trigger(context.Background(), types.CHUNK_RERANK, chatManage))
	require.Len(t, chatManage.RerankResult, 1)
	require.Equal(t, "web-high-relevance", chatManage.RerankResult[0].ID)
	require.Equal(t, 0.87, chatManage.RerankResult[0].Score)
	require.Equal(t, "0.2000", chatManage.RerankResult[0].Metadata["base_score"])
	require.Equal(t, "0.8700", chatManage.RerankResult[0].Metadata["model_score"])
}

func TestEmployeeSemanticRejectionDoesNotFallBackToRawCandidate(t *testing.T) {
	plugin := &PluginMerge{}
	raw := []*types.SearchResult{{ID: "weak", Content: "unrelated candidate", Score: 0.2}}
	employee := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{EmployeeAssistant: true},
		PipelineState: types.PipelineState{
			RewriteQuery:   "unrelated candidate",
			SearchResult:   raw,
			RerankExecuted: true,
			History: []*types.History{{
				Query: "old conditions",
				KnowledgeReferences: []*types.SearchResult{{
					ID: "historic", Content: "unrelated candidate", Score: 0.9,
				}},
			}},
		},
	}
	require.Nil(t, plugin.OnEvent(context.Background(), types.CHUNK_MERGE, employee, func() *PluginError { return nil }))
	require.Empty(t, employee.MergeResult)

	generic := &types.ChatManage{PipelineState: types.PipelineState{SearchResult: raw, RerankExecuted: true}}
	require.Equal(t, raw, plugin.selectInputResults(context.Background(), generic), "generic legacy fallback is unchanged")
}

func TestEmployeeRerankResolvesActiveDefaultForCandidates(t *testing.T) {
	model := &fixedScoreReranker{results: []rerank.RankResult{{Index: 0, RelevanceScore: 0.9}}}
	service := &rerankModelService{
		model: model,
		models: []*types.Model{
			{ID: "active", Type: types.ModelTypeRerank, Status: types.ModelStatusActive},
			{ID: "default", Type: types.ModelTypeRerank, Status: types.ModelStatusActive, IsDefault: true},
		},
	}
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{EmployeeAssistant: true, RerankThreshold: 0.3},
		PipelineState: types.PipelineState{
			RewriteQuery: "current question",
			SearchResult: []*types.SearchResult{{ID: "candidate", Content: "current evidence"}},
		},
	}
	manager := NewEventManager()
	NewPluginRerank(manager, service)
	require.Nil(t, manager.Trigger(context.Background(), types.CHUNK_RERANK, cm))
	require.Equal(t, "default", cm.RerankModelID)
	require.True(t, cm.RerankExecuted)
	require.Len(t, cm.RerankResult, 1)
	require.Equal(t, "candidate", cm.RerankResult[0].ID)
	require.Equal(t, 0.9, cm.RerankResult[0].Score)
	require.Equal(t, 1, service.listCalls)
	require.Equal(t, 1, service.getCalls)
}

func TestEmployeeNoRetrievalDoesNotResolveRerankModel(t *testing.T) {
	service := &rerankModelService{}
	manager := NewEventManager()
	NewPluginRerank(manager, service)
	needed := false
	chatManage := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{EmployeeAssistant: true},
		PipelineState: types.PipelineState{
			RetrievalNeeded: &needed,
			SearchResult:    []*types.SearchResult{{ID: "unused", Content: "unused"}},
		},
	}
	require.Nil(t, manager.Trigger(context.Background(), types.CHUNK_RERANK, chatManage))
	require.Zero(t, service.listCalls)
	require.Zero(t, service.getCalls)
}
