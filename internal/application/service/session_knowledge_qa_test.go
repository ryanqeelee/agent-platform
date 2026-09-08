package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureChatModel struct {
	lastMessages []chat.Message
}

func (m *captureChatModel) Chat(
	context.Context,
	[]chat.Message,
	*chat.ChatOptions,
) (*types.ChatResponse, error) {
	return nil, nil
}

func (m *captureChatModel) ChatStream(
	_ context.Context,
	messages []chat.Message,
	_ *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	m.lastMessages = append([]chat.Message(nil), messages...)

	ch := make(chan types.StreamResponse, 1)
	ch <- types.StreamResponse{
		ResponseType: types.ResponseTypeAnswer,
		Content:      "ok",
		Done:         true,
	}
	close(ch)
	return ch, nil
}

func (m *captureChatModel) GetModelName() string { return "capture" }
func (m *captureChatModel) GetModelID() string   { return "capture" }

type stubModelService struct {
	chatModel       chat.Chat
	modelsByID      map[string]*types.Model
	availableModels []*types.Model
}

func TestEmitKnowledgeReferencesEventIgnoresCitationOutputSetting(t *testing.T) {
	bus := event.NewEventBus()
	var emitted []event.Event
	bus.On(event.EventAgentReferences, func(_ context.Context, evt event.Event) error {
		emitted = append(emitted, evt)
		return nil
	})
	disabled := false
	result := &types.SearchResult{ID: "chunk-1", KnowledgeTitle: "Doc"}
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{CitationEnabled: &disabled},
		PipelineState: types.PipelineState{
			MergeResult: []*types.SearchResult{result},
		},
		PipelineContext: types.PipelineContext{EventBus: bus.AsEventBusInterface()},
	}

	emitKnowledgeReferencesEvent(context.Background(), cm)
	require.Len(t, emitted, 1)
	require.Equal(t, event.EventAgentReferences, emitted[0].Type)
	require.Equal(t, []*types.SearchResult{result}, emitted[0].Data.(event.AgentReferencesData).References)

	enabled := true
	cm.CitationEnabled = &enabled
	emitKnowledgeReferencesEvent(context.Background(), cm)
	require.Len(t, emitted, 2)
}

func (s *stubModelService) CreateModel(context.Context, *types.Model) error {
	return nil
}

func (s *stubModelService) GetModelByID(_ context.Context, id string) (*types.Model, error) {
	return s.modelsByID[id], nil
}

func (s *stubModelService) ListModels(context.Context) ([]*types.Model, error) {
	if s.availableModels != nil {
		return s.availableModels, nil
	}
	models := make([]*types.Model, 0, len(s.modelsByID))
	for _, model := range s.modelsByID {
		models = append(models, model)
	}
	return models, nil
}

func (s *stubModelService) UpdateModel(context.Context, *types.Model) error {
	return nil
}

func (s *stubModelService) DeleteModel(context.Context, string) error {
	return nil
}

func (s *stubModelService) UpdateModelCredentials(
	context.Context, string, *string, *string,
) (*types.Model, error) {
	return nil, nil
}

func (s *stubModelService) ClearModelCredential(context.Context, string, string) error {
	return nil
}

func (s *stubModelService) GetEmbeddingModel(context.Context, string) (embedding.Embedder, error) {
	return nil, nil
}

func (s *stubModelService) GetEmbeddingModelForTenant(context.Context, string, uint64) (embedding.Embedder, error) {
	return nil, nil
}

func (s *stubModelService) GetRerankModel(context.Context, string) (rerank.Reranker, error) {
	return nil, nil
}

func (s *stubModelService) GetChatModel(context.Context, string) (chat.Chat, error) {
	return s.chatModel, nil
}

func (s *stubModelService) GetVLMModel(context.Context, string) (vlm.VLM, error) {
	return nil, nil
}

func (s *stubModelService) GetASRModel(context.Context, string) (asr.ASR, error) {
	return nil, nil
}

func TestHandleModelFallback_IncludesHistoryMessages(t *testing.T) {
	chatModel := &captureChatModel{}
	svc := &sessionService{
		modelService: &stubModelService{chatModel: chatModel},
	}

	bus := event.NewEventBus()
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			SessionID:      "session-1",
			Query:          "现在还能继续讲吗？",
			ChatModelID:    "chat-model",
			FallbackPrompt: "Answer the latest user question: {{query}}",
			SummaryConfig: types.SummaryConfig{
				Temperature: 0.2,
			},
			Language: "zh-CN",
		},
		PipelineState: types.PipelineState{
			History: []*types.History{
				{
					Query:  "先介绍一下 WeKnora",
					Answer: "WeKnora 是一个知识库问答系统。",
				},
			},
		},
		PipelineContext: types.PipelineContext{
			EventBus: bus.AsEventBusInterface(),
		},
	}

	svc.handleModelFallback(context.Background(), cm)

	// Corrected fallback shape: a system message carries the fallback
	// instruction, history is replayed in the middle, and the turn ends on the
	// user's question. Previously the system message was dropped entirely.
	require.Len(t, chatModel.lastMessages, 4)
	assert.Equal(t, "system", chatModel.lastMessages[0].Role)
	assert.Contains(t, chatModel.lastMessages[0].Content, "Answer the latest user question")
	assert.Equal(t, "user", chatModel.lastMessages[1].Role)
	assert.Equal(t, "先介绍一下 WeKnora", chatModel.lastMessages[1].Content)
	assert.Equal(t, "assistant", chatModel.lastMessages[2].Role)
	assert.Equal(t, "WeKnora 是一个知识库问答系统。", chatModel.lastMessages[2].Content)
	assert.Equal(t, "user", chatModel.lastMessages[3].Role)
	assert.Contains(t, chatModel.lastMessages[3].Content, "现在还能继续讲吗？")
}

func TestEmployeeQuickEmptyScopeUsesQueryUnderstandingPipeline(t *testing.T) {
	employeeQuick := &types.CustomAgent{
		ID:     types.BuiltinEmployeeAssistantID,
		Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer},
	}

	pipeline, needsPipelineDecision := buildKnowledgeQAPipeline(false, false, false, false, employeeQuick)
	require.True(t, needsPipelineDecision)
	require.Equal(t, []types.EventType{
		types.MEMORY_RECALL,
		types.QUERY_UNDERSTAND,
		types.CHUNK_SEARCH_PARALLEL,
		types.CHUNK_RERANK,
		types.CHUNK_MERGE,
		types.FILTER_TOP_K,
		types.INTO_CHAT_MESSAGE,
		types.CHAT_COMPLETION_STREAM,
	}, pipeline)

	ordinaryPipeline, ordinaryNeedsDecision := buildKnowledgeQAPipeline(
		false, false, false, false,
		&types.CustomAgent{ID: "ordinary-agent", Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer}},
	)
	require.False(t, ordinaryNeedsDecision)
	require.Equal(t, []types.EventType{types.MEMORY_RECALL, types.CHAT_COMPLETION_STREAM}, ordinaryPipeline)

	withKBPipeline, withKBNeedsDecision := buildKnowledgeQAPipeline(true, false, false, false, nil)
	require.True(t, withKBNeedsDecision)
	require.Contains(t, withKBPipeline, types.QUERY_UNDERSTAND)
}

func TestHandleModelFallback_EvidenceBoundOverrideRetainsCurrentAttachments(t *testing.T) {
	chatModel := &captureChatModel{}
	svc := &sessionService{modelService: &stubModelService{chatModel: chatModel}}
	bus := event.NewEventBus()
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			SessionID:               "session-1",
			Query:                   "what is the rule?",
			ChatModelID:             "chat-model",
			FallbackPrompt:          "GENERIC: use general knowledge and the KB catalog",
			Language:                "English",
			ChatModelSupportsVision: true,
			Images:                  []string{"https://example.com/current.png"},
			Attachments: types.MessageAttachments{{
				FileName: "current.txt",
				FileType: ".txt",
				Content:  "attachment-supported fact",
			}},
		},
		PipelineState: types.PipelineState{
			RewriteQuery:         "rewritten rule question",
			SystemPromptOverride: "OVERRIDE {{language}}: {{query}}",
			QuotedContext:        "current quoted material",
			History: []*types.History{{
				Query:  "earlier question",
				Answer: "earlier answer",
			}},
		},
		PipelineContext: types.PipelineContext{EventBus: bus.AsEventBusInterface()},
	}

	svc.handleModelFallback(context.Background(), cm)

	require.Len(t, chatModel.lastMessages, 4)
	system := chatModel.lastMessages[0]
	require.Equal(t, "system", system.Role)
	assert.Contains(t, system.Content, "OVERRIDE English: rewritten rule question")
	assert.Contains(t, system.Content, "no matching authorized knowledge-base or search evidence was obtained this turn")
	assert.NotContains(t, system.Content, "GENERIC: use general knowledge")
	assert.Contains(t, system.Content, "Do not use general knowledge")

	assert.Equal(t, "earlier question", chatModel.lastMessages[1].Content)
	assert.Equal(t, "earlier answer", chatModel.lastMessages[2].Content)
	user := chatModel.lastMessages[3]
	assert.Equal(t, "user", user.Role)
	assert.Contains(t, user.Content, "rewritten rule question")
	assert.Contains(t, user.Content, "current quoted material")
	assert.Contains(t, user.Content, "attachment-supported fact")
	assert.Equal(t, []string{"https://example.com/current.png"}, user.Images)
}

func TestHandleModelFallback_EvidenceBoundOverrideWorksWithoutGenericPrompt(t *testing.T) {
	chatModel := &captureChatModel{}
	svc := &sessionService{modelService: &stubModelService{chatModel: chatModel}}
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			Query:       "question",
			ChatModelID: "chat-model",
			Language:    "English",
		},
		PipelineState:   types.PipelineState{SystemPromptOverride: "evidence-first override"},
		PipelineContext: types.PipelineContext{EventBus: event.NewEventBus().AsEventBusInterface()},
	}

	svc.handleModelFallback(context.Background(), cm)

	require.NotEmpty(t, chatModel.lastMessages)
	assert.Contains(t, chatModel.lastMessages[0].Content, "evidence-first override")
}
