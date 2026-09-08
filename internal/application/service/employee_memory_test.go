package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type employeeMemoryModel struct {
	fakeAgentChatModel
	messages []chat.Message
}

func (m *employeeMemoryModel) ChatStream(ctx context.Context, messages []chat.Message, options *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.messages = messages
	return m.fakeAgentChatModel.ChatStream(ctx, messages, options)
}

type employeeMemoryModels struct {
	employeeModelsStub
	model *employeeMemoryModel
}

func (m employeeMemoryModels) GetChatModel(context.Context, string) (chat.Chat, error) {
	return m.model, nil
}
func (m employeeMemoryModels) GetModelByID(context.Context, string) (*types.Model, error) {
	return &types.Model{}, nil
}

type employeeMemoryMessages struct{ interfaces.MessageRepository }

func (employeeMemoryMessages) GetSessionAttachments(context.Context, string) (types.MessageAttachments, error) {
	return nil, nil
}
func (employeeMemoryMessages) GetRecentMessagesBySession(context.Context, string, int) ([]*types.Message, error) {
	return nil, nil
}

type employeeRecallMemory struct {
	stubMemoryAvailability
	calls int
}

func (m *employeeRecallMemory) Recall(ctx context.Context, _ string) interfaces.MemoryRecall {
	m.calls++
	if !m.MemoryAvailable(ctx) {
		return interfaces.MemoryRecall{}
	}
	return interfaces.MemoryRecall{Prompt: "项目代号青鹭7392", Items: []*types.MemoryItem{{ID: "test-memory", Content: "项目代号青鹭7392"}}}
}

func TestEmployeeAgentQAInjectsMemoryIntoModelContext(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "agent disabled"}[enabled], func(t *testing.T) {
			model := &employeeMemoryModel{}
			memory := &employeeRecallMemory{stubMemoryAvailability: stubMemoryAvailability{available: true}}
			ctx := context.WithValue(t.Context(), types.TenantIDContextKey, uint64(10004))
			ctx = context.WithValue(ctx, types.TenantInfoContextKey, &types.Tenant{ID: 10004})
			svc := &sessionService{cfg: &config.Config{}, modelService: employeeMemoryModels{model: model}, messageRepo: employeeMemoryMessages{},
				webSearchProviderRepo: &agentMemoryWebSearchRepo{}, memoryService: memory, agentService: &agentService{memoryService: memory},
				scenarioCapabilities: assistantScenarioResolverStub{settings: &types.AssistantScenarioCapabilitySettings{
					Capabilities: types.AssistantScenarioCapabilities{Tools: true},
				}},
			}
			bus := event.NewEventBus()
			recalled := false
			bus.On(event.EventMemoryRecalled, func(context.Context, event.Event) error { recalled = true; return nil })
			req := &types.QARequest{Session: &types.Session{ID: "new-session", TenantID: 10004}, Query: "我的项目代号是什么？", AssistantMessageID: "reply",
				CustomAgent: &types.CustomAgent{ID: types.BuiltinEmployeeAssistantID, TenantID: 10004, Config: types.CustomAgentConfig{
					AgentMode: types.AgentModeSmartReasoning, KBSelectionMode: "none", SkillsSelectionMode: "none", MCPSelectionMode: "none", MemoryEnabled: &enabled,
				}},
			}
			require.NoError(t, svc.AgentQA(ctx, req, bus))
			require.Equal(t, 1, memory.calls)
			require.NotEmpty(t, model.messages)
			if enabled {
				require.Contains(t, model.messages[0].Content, "青鹭7392")
			} else {
				require.NotContains(t, model.messages[0].Content, "青鹭7392")
			}
			require.Equal(t, enabled, recalled)
		})
	}
}
