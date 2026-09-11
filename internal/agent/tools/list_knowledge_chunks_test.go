package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/modelcontext"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type windowChunkRepo struct {
	interfaces.ChunkRepository
	chunks   []*types.Chunk
	calls    int
	failPage int
}

func (r *windowChunkRepo) ListPagedChunksByKnowledgeID(_ context.Context, tenant uint64, id string, p *types.Pagination, kinds []types.ChunkType, _ []string, _, _, _, _ string, enabled *bool) ([]*types.Chunk, int64, error) {
	r.calls++
	if tenant != 1 || id != "doc" || enabled == nil || !*enabled || len(kinds) != 2 {
		panic("lost authorized scope or filters")
	}
	if p.Page == r.failPage {
		return nil, 0, fmt.Errorf("read failed")
	}
	start := min(p.Offset(), len(r.chunks))
	end := min(start+p.Limit(), len(r.chunks))
	return r.chunks[start:end], int64(len(r.chunks)), nil
}
func (r *windowChunkRepo) ListChunksByParentIDs(context.Context, uint64, []string) ([]*types.Chunk, error) {
	return nil, nil
}

type windowChunkService struct {
	interfaces.ChunkService
	repo *windowChunkRepo
}

func (s *windowChunkService) GetRepository() interfaces.ChunkRepository { return s.repo }

func TestListKnowledgeChunksExactWindow(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		total, offset, limit, want, calls int
		more, success                     bool
	}{
		{"first", 173, 0, 100, 100, 1, true, true},
		{"unaligned", 250, 120, 100, 100, 2, true, true},
		{"unaligned tail", 173, 120, 100, 53, 1, false, true},
		{"aligned tail", 173, 160, 40, 13, 1, false, true},
		{"empty", 0, 0, 20, 0, 1, false, true},
		{"past end inside repository page", 173, 180, 100, 0, 1, false, false},
		{"past end", 173, 220, 100, 0, 1, false, false},
		{"bounded limit", 250, 0, 200, 100, 1, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &windowChunkRepo{}
			for i := 0; i < tc.total; i++ {
				repo.chunks = append(repo.chunks, &types.Chunk{ID: fmt.Sprintf("chunk-%d", i), KnowledgeID: "doc", KnowledgeBaseID: "kb", ChunkIndex: i * 2, Content: fmt.Sprintf("text-%d", i), ChunkType: types.ChunkTypeText})
			}
			tool := NewListKnowledgeChunksTool(&scopeKnowledgeService{knowledge: &types.Knowledge{ID: "doc", KnowledgeBaseID: "kb", TenantID: 1, CustomMetadata: types.JSON(`{"region":"North","authority":"draft"}`), Metadata: types.JSON(`{"external_id":"secret"}`)}}, &windowChunkService{repo: repo}, types.SearchTargets{{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb", TenantID: 1}})
			args, _ := json.Marshal(ListKnowledgeChunksInput{KnowledgeID: "doc", Offset: tc.offset, Limit: tc.limit})
			result, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			require.Equal(t, tc.success, result.Success)
			require.Equal(t, tc.calls, repo.calls)
			if !tc.success {
				require.Contains(t, result.Error, "out of range")
				return
			}
			require.Equal(t, tc.want, result.Data["fetched_chunks"])
			require.Equal(t, tc.offset+tc.want, result.Data["next_offset"])
			require.Equal(t, tc.more, result.Data["has_more"])
			chunks := result.Data["chunks"].([]map[string]interface{})
			for i, c := range chunks {
				require.Equal(t, fmt.Sprintf("chunk-%d", tc.offset+i), c["chunk_id"])
			}
			require.Contains(t, result.Output, fmt.Sprintf(`remaining="%d" has_more="%t"`, tc.total-tc.offset-tc.want, tc.more))
			modelOutput := modelcontext.NewRegistry(true).ModelToolResultForTool(ToolListKnowledgeChunks, result)
			if tc.want > 0 {
				require.Contains(t, modelOutput, "authority: draft")
				require.Contains(t, modelOutput, "region: North")
			}
			require.NotContains(t, modelOutput, "external_id")
			require.Contains(t, modelOutput, fmt.Sprintf(`next_offset="%d"`, tc.offset+tc.want))
			require.Contains(t, modelOutput, fmt.Sprintf(`has_more="%t"`, tc.more))
			require.Contains(t, modelOutput, fmt.Sprintf(`remaining="%d"`, tc.total-tc.offset-tc.want))
		})
	}
}

func TestListKnowledgeChunksSecondPageFailure(t *testing.T) {
	repo := &windowChunkRepo{chunks: make([]*types.Chunk, 250), failPage: 3}
	for i := range repo.chunks {
		repo.chunks[i] = &types.Chunk{}
	}
	tool := NewListKnowledgeChunksTool(&scopeKnowledgeService{knowledge: &types.Knowledge{ID: "doc", KnowledgeBaseID: "kb", TenantID: 1}}, &windowChunkService{repo: repo}, types.SearchTargets{{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb", TenantID: 1}})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_id":"doc","offset":120,"limit":100}`))
	require.ErrorContains(t, err, "read failed")
	require.False(t, result.Success)
	require.Empty(t, result.Output)
}

func TestSingleChunkOutputDoesNotClaimDocumentCoverage(t *testing.T) {
	output := (&ListKnowledgeChunksTool{}).buildOutput("doc", "Document", 1, 0, []*types.Chunk{{ID: "chunk", Content: "evidence"}}, true)
	require.Contains(t, output, `single_chunk="true"`)
	require.NotContains(t, output, `total=`)
	require.NotContains(t, output, "pagination")
}

func TestGetDocumentInfoProjectsOnlyUserAuthoredMetadata(t *testing.T) {
	knowledge := &types.Knowledge{
		ID: "doc", KnowledgeBaseID: "kb", TenantID: 1, Title: "Policy",
		CustomMetadata: types.JSON(`{"authority":"draft","region":"North"}`),
		Metadata:       types.JSON(`{"external_id":"internal-ingestion-id"}`),
	}
	repo := &windowChunkRepo{chunks: []*types.Chunk{{ID: "chunk", KnowledgeID: "doc", ChunkType: types.ChunkTypeText}}}
	tool := NewGetDocumentInfoTool(
		&scopeKnowledgeService{knowledge: knowledge},
		&windowChunkService{repo: repo},
		types.SearchTargets{{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb", TenantID: 1}},
	)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_ids":["doc"]}`))
	require.NoError(t, err)
	require.Contains(t, result.Output, "authority: draft")
	require.NotContains(t, result.Output, "external_id")
	modelOutput := modelcontext.NewRegistry(true).ModelToolResultForTool(ToolGetDocumentInfo, result)
	require.Contains(t, modelOutput, "region: North")
	require.NotContains(t, modelOutput, "internal-ingestion-id")
}

func TestKnowledgeSearchFormattingDoesNotQueryDocumentTotals(t *testing.T) {
	// No repository: search rendering must not issue count queries for matched documents.
	tool := &KnowledgeSearchTool{seenChunks: map[string]bool{}, searchTargets: types.SearchTargets{{KnowledgeBaseID: "kb", TenantID: 1}}}
	result, err := tool.formatOutput(context.Background(), []*searchResultWithMeta{{SearchResult: &types.SearchResult{ID: "chunk", KnowledgeID: "doc", KnowledgeTitle: "Source", Content: "evidence"}, KnowledgeBaseID: "kb"}}, []string{"kb"}, []string{"question"})
	require.NoError(t, err)
	require.Contains(t, result.Output, "evidence")
	require.Contains(t, result.Output, `knowledge_id="doc"`)
	require.NotContains(t, result.Output, "coverage")
	require.NotContains(t, result.Output, "remaining")
	require.Equal(t, 1, result.Data["count"])
}
