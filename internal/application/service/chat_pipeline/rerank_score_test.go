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
	model rerank.Reranker
}

func (s *rerankModelService) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return s.model, nil
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
