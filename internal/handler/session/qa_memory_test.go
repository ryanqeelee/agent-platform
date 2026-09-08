package session

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service/memory"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type turnMemoryProbe struct {
	interfaces.MemoryService
	scope     interfaces.MemoryScope
	allowed   bool
	extracted bool
	modelID   string
}
type memoryTestStreamManager struct{ terminalStreamManager }

func (*memoryTestStreamManager) GetEvents(context.Context, string, string, int) ([]interfaces.StreamEvent, int, error) {
	return []interfaces.StreamEvent{{Type: types.ResponseTypeComplete, Done: true}}, 1, nil
}
func (p *turnMemoryProbe) Remember(ctx context.Context, item types.MemoryItem) (*types.MemoryItem, error) {
	var err error
	p.scope, err = memory.ResolveScope(ctx)
	p.allowed = types.MemoryAllowedForAgent(ctx)
	return &item, err
}
func (p *turnMemoryProbe) Snapshot(ctx context.Context, consumer string) (*types.PersonalMemorySnapshot, error) {
	p.allowed = types.MemoryAllowedForAgent(ctx)
	if !p.allowed {
		return &types.PersonalMemorySnapshot{Status: "disabled"}, nil
	}
	return &types.PersonalMemorySnapshot{
		Status: "available", Consumer: consumer,
		Policy: types.PersonalMemoryPolicy{WorkspaceEnabled: true, UserEnabled: true, WriteMode: types.MemoryWriteAuto},
	}, nil
}
func (p *turnMemoryProbe) SubmitExpression(ctx context.Context, expression *types.PersonalMemoryExpression) (*types.PersonalMemoryExpressionReceipt, error) {
	p.extracted = types.MemoryAllowedForAgent(ctx)
	return &types.PersonalMemoryExpressionReceipt{Status: types.MemoryExpressionAccepted}, nil
}
func (p *turnMemoryProbe) SubmitExpressionWithModel(ctx context.Context, expression *types.PersonalMemoryExpression, modelID string) (*types.PersonalMemoryExpressionReceipt, error) {
	p.modelID = modelID
	return p.SubmitExpression(ctx, expression)
}

func TestEmployeeTurnMemoryPreservesCallerAndPreference(t *testing.T) {
	require.NoError(t, types.LoadBuiltinAgentsConfig(filepath.Join("..", "..", "..", "config")))
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "enabled", true: "agent disabled"}[disabled], func(t *testing.T) {
			ctx := context.WithValue(t.Context(), types.TenantIDContextKey, uint64(10004))
			ctx = types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalWebUser, ID: "memory-test"})
			ctx = logger.CloneContext(ctx)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			probe := &turnMemoryProbe{}
			h := &Handler{streamManager: &memoryTestStreamManager{}, memoryService: probe}
			builtin := types.GetBuiltinAgentWithContext(ctx, types.BuiltinEmployeeAssistantID, 10004)
			require.NotNil(t, builtin)
			if disabled {
				off := false
				builtin.Config.MemoryEnabled = &off
			}
			reqCtx := &qaRequestContext{
				ctx: ctx, c: c, sessionID: "memory-session", session: &types.Session{ID: "memory-session", TenantID: 10004},
				customAgent:      builtin,
				assistantMessage: &types.Message{ID: "reply", SessionID: "memory-session"},
			}
			reqCtx.ctx = h.captureTurnMemoryInput(
				reqCtx.ctx, reqCtx.session, reqCtx.customAgent, "请记住：测试代号青鹭7392", "user-message", "chat-model",
			)
			stream := h.setupSSEStream(reqCtx, false)
			defer stream.cancel()
			h.recordTurnMemory(context.WithoutCancel(stream.asyncCtx), stream.assistantMessage, "请记住：测试代号青鹭7392", "user-message")
			if disabled {
				require.Equal(t, interfaces.MemoryScope{}, probe.scope)
			} else {
				require.Equal(t, interfaces.MemoryScope{TenantID: 10004, SubjectID: "web_user:memory-test"}, probe.scope)
			}
			require.Equal(t, !disabled, probe.allowed)
			require.Equal(t, !disabled, probe.extracted)
			if !disabled {
				require.Equal(t, "chat-model", probe.modelID)
			}
		})
	}
}
