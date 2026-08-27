package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type revokedSuggestionAgentRepo struct {
	interfaces.CustomAgentRepository
}

func (revokedSuggestionAgentRepo) GetAgentByID(context.Context, string, uint64) (*types.CustomAgent, error) {
	return &types.CustomAgent{ID: "agent", TenantID: 1, Config: types.CustomAgentConfig{
		KBSelectionMode: "none",
		QuestionSuggestions: &types.QuestionSuggestionConfig{Starters: types.StarterSuggestionConfig{
			Enabled: true, Mode: types.SuggestionModeKnowledge, Count: 3,
		}},
	}}, nil
}

type revokedSuggestionKBService struct {
	interfaces.KnowledgeBaseService
}

func (revokedSuggestionKBService) GetKnowledgeBasesByIDsOnly(context.Context, []string) ([]*types.KnowledgeBase, error) {
	return []*types.KnowledgeBase{{ID: "kb", TenantID: 1}}, nil
}

func (revokedSuggestionKBService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return nil, fmt.Errorf("business role grant revoked")
}

type revokedSuggestionKnowledgeRepo struct{ interfaces.KnowledgeRepository }

func (revokedSuggestionKnowledgeRepo) GetKnowledgeByIDOnly(context.Context, string) (*types.Knowledge, error) {
	return &types.Knowledge{ID: "doc", TenantID: 1, KnowledgeBaseID: "kb"}, nil
}

func TestSuggestedQuestionsHonorCurrentBusinessRoleForExplicitScope(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	svc := &customAgentService{
		repo: revokedSuggestionAgentRepo{}, kbService: revokedSuggestionKBService{},
		knowledgeRepo: revokedSuggestionKnowledgeRepo{},
	}

	for _, tc := range []struct {
		name         string
		kbIDs        []string
		knowledgeIDs []string
	}{
		{name: "knowledge base", kbIDs: []string{"kb"}},
		{name: "document", knowledgeIDs: []string{"doc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.GetSuggestedQuestions(ctx, "agent", tc.kbIDs, tc.knowledgeIDs, nil, 3)
			require.NoError(t, err)
			require.Empty(t, got)
		})
	}
}
