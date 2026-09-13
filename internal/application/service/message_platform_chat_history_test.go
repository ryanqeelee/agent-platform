package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type platformMessageKBStub struct {
	interfaces.KnowledgeBaseService
	results     map[string][]*types.SearchResult
	searchedKBs []string
	ensureCalls int
}

func (s *platformMessageKBStub) EnsureChatHistoryKnowledgeBase(context.Context, string) (*types.KnowledgeBase, error) {
	s.ensureCalls++
	return &types.KnowledgeBase{ID: "unexpected"}, nil
}

func (s *platformMessageKBStub) HybridSearch(
	_ context.Context, id string, _ types.SearchParams,
) ([]*types.SearchResult, error) {
	s.searchedKBs = append(s.searchedKBs, id)
	return s.results[id], nil
}

type platformMessageRepoStub struct {
	interfaces.MessageRepository
	messages map[string]*types.MessageWithSession
	owned    map[uint64]map[string]bool
}

func (s *platformMessageRepoStub) GetMessagesByKnowledgeIDs(
	_ context.Context, ids []string,
) ([]*types.MessageWithSession, error) {
	result := make([]*types.MessageWithSession, 0, len(ids))
	for _, id := range ids {
		if message := s.messages[id]; message != nil {
			result = append(result, message)
		}
	}
	return result, nil
}

func (s *platformMessageRepoStub) OwnedSessionIDs(
	_ context.Context, tenantID uint64, _ string, sessionIDs []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(sessionIDs))
	for _, id := range sessionIDs {
		result[id] = s.owned[tenantID][id]
	}
	return result, nil
}

func (*platformMessageRepoStub) GetMessagesByRequestIDs(context.Context, []string) ([]*types.MessageWithSession, error) {
	return nil, nil
}

func messageSearchContext(tenantID uint64, userID string) context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
	return context.WithValue(ctx, types.UserIDContextKey, userID)
}

func TestMessageSearchUsesTenantPrivateKBAndUserOwnership(t *testing.T) {
	policyRepo := &platformChatHistoryRepoStub{
		config: types.PlatformChatHistoryConfig{Enabled: true, EmbeddingModelID: "embed-platform"},
		bindings: map[uint64]*types.KnowledgeBase{
			10001: {ID: "tenant-one-kb", TenantID: 10001, EmbeddingModelID: "embed-platform"},
			10004: {ID: "tenant-two-kb", TenantID: 10004, EmbeddingModelID: "embed-platform"},
		},
	}
	kbService := &platformMessageKBStub{results: map[string][]*types.SearchResult{
		"tenant-one-kb": {
			{KnowledgeID: "tenant-one-alice", Score: 0.9},
			{KnowledgeID: "tenant-one-bob", Score: 0.8},
		},
		"tenant-two-kb": {{KnowledgeID: "tenant-two-alice", Score: 0.95}},
	}}
	messageRepo := &platformMessageRepoStub{
		messages: map[string]*types.MessageWithSession{
			"tenant-one-alice": {Message: types.Message{ID: "m1", SessionID: "s1", KnowledgeID: "tenant-one-alice", Role: "assistant", Content: "tenant one"}},
			"tenant-one-bob":   {Message: types.Message{ID: "m2", SessionID: "s-bob", KnowledgeID: "tenant-one-bob", Role: "assistant", Content: "private coworker"}},
			"tenant-two-alice": {Message: types.Message{ID: "m3", SessionID: "s2", KnowledgeID: "tenant-two-alice", Role: "assistant", Content: "tenant two"}},
		},
		owned: map[uint64]map[string]bool{
			10001: {"s1": true},
			10004: {"s2": true},
		},
	}
	messageService := &messageService{
		messageRepo:         messageRepo,
		kbService:           kbService,
		platformChatHistory: NewPlatformChatHistoryService(policyRepo, nil),
	}

	first, err := messageService.SearchMessages(messageSearchContext(10001, "alice"), &types.MessageSearchParams{
		Query: "query", Mode: types.MessageSearchModeVector,
	})
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	require.Equal(t, "tenant one", first.Items[0].AnswerContent)

	second, err := messageService.SearchMessages(messageSearchContext(10004, "alice"), &types.MessageSearchParams{
		Query: "query", Mode: types.MessageSearchModeVector,
	})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, "tenant two", second.Items[0].AnswerContent)
	require.Equal(t, []string{"tenant-one-kb", "tenant-two-kb"}, kbService.searchedKBs)
}

func TestPlatformPolicyOffSkipsNewMessageIndex(t *testing.T) {
	policyRepo := &platformChatHistoryRepoStub{
		config:   types.PlatformChatHistoryConfig{Enabled: false},
		bindings: map[uint64]*types.KnowledgeBase{},
	}
	kbService := &platformMessageKBStub{}
	messageService := &messageService{
		kbService:           kbService,
		platformChatHistory: NewPlatformChatHistoryService(policyRepo, nil),
	}
	messageService.IndexMessageToKB(messageSearchContext(10001, "alice"), "question", "answer", "message", "session")
	require.Zero(t, kbService.ensureCalls)
}
