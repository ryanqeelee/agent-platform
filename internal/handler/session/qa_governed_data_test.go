package session

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

func TestGovernedDataInteractiveDetach(t *testing.T) {
	requestCtx := context.WithValue(context.Background(), types.UserIDContextKey, "user-a")
	requestCtx = context.WithValue(requestCtx, types.TenantIDContextKey, uint64(10001))
	requestCtx = types.WithGovernedDataUserCredential(requestCtx, "private-jwt")

	ordinary := cloneInteractiveQATurn(requestCtx)
	if types.GovernedDataObservability(ordinary) {
		t.Fatal("ordinary QA turn was marked governed")
	}
	if _, _, ok := types.GovernedDataUserCredential(ordinary); ok {
		t.Fatal("ordinary QA turn inherited governed credential")
	}
	historyOnly := types.WithGovernedDataObservability(ordinary)
	historyAsync := cloneInteractiveQATurn(historyOnly)
	if !types.GovernedDataObservability(historyAsync) {
		t.Fatal("governed history lost its observability restriction")
	}
	if _, _, ok := types.GovernedDataUserCredential(historyAsync); ok {
		t.Fatal("governed history copied a credential into an ordinary agent turn")
	}

	parsed := types.WithGovernedDataObservability(logger.CloneContext(requestCtx))
	parsed = types.CopyGovernedDataTurnCredential(parsed, requestCtx)
	async, cancel := context.WithCancel(cloneInteractiveQATurn(parsed))
	defer cancel()
	token, tenant, ok := types.GovernedDataUserCredential(async)
	if !ok || token != "private-jwt" || tenant != 10001 {
		t.Fatal("QA turn lost authenticated credential")
	}
	if _, _, ok := types.GovernedDataUserCredential(logger.CloneContext(requestCtx)); ok {
		t.Fatal("generic worker inherited credential")
	}
	shared := context.WithValue(parsed, types.TenantIDContextKey, uint64(20002))
	if _, _, ok := types.GovernedDataUserCredential(cloneInteractiveQATurn(shared)); ok {
		t.Fatal("shared agent inherited business authority")
	}
	cancel()
	if async.Err() != context.Canceled {
		t.Fatal("native turn cancellation lost")
	}
}

type governedHistoryMessageService struct {
	interfaces.MessageService
	messages []*types.Message
	err      error
	limit    int
}

func (s *governedHistoryMessageService) GetRecentMessagesBySession(
	_ context.Context, _ string, limit int,
) ([]*types.Message, error) {
	s.limit = limit
	return s.messages, s.err
}

func TestGovernedTurnClassificationUsesEffectiveToolsAndReplayableHistory(t *testing.T) {
	governedAgent := &types.CustomAgent{Config: types.CustomAgentConfig{
		AllowedTools: []string{tools.ToolGovernedDataSchema, tools.ToolGovernedDataQuery},
	}}
	require.True(t, agentCanConsumeGovernedData(governedAgent))
	require.False(t, agentCanConsumeGovernedData(&types.CustomAgent{}))

	history := &governedHistoryMessageService{messages: []*types.Message{{
		Role: "assistant",
		AgentSteps: types.AgentSteps{{ToolCalls: []types.ToolCall{{
			Name: tools.ToolGovernedDataQuery,
		}}}},
	}}}
	h := &Handler{messageService: history}
	ordinaryAgent := &types.CustomAgent{Config: types.CustomAgentConfig{
		MultiTurnEnabled: true,
		HistoryTurns:     3,
	}}
	hasHistory, err := h.sessionHasGovernedHistory(context.Background(), "session-1", ordinaryAgent, true)
	require.NoError(t, err)
	require.True(t, hasHistory)
	require.Equal(t, 50, history.limit)

	ordinaryAgent.Config.HistoryTurns = 0
	hasHistory, err = h.sessionHasGovernedHistory(context.Background(), "session-1", ordinaryAgent, true)
	require.NoError(t, err)
	require.True(t, hasHistory)
	require.Equal(t, 50, history.limit)

	ordinaryAgent.Config.MultiTurnEnabled = false
	hasHistory, err = h.sessionHasGovernedHistory(context.Background(), "session-1", ordinaryAgent, true)
	require.NoError(t, err)
	require.False(t, hasHistory)
}

type governedAdmissionUsers struct{ interfaces.UserService }

func (governedAdmissionUsers) ValidateToken(context.Context, string) (*types.User, uint64, error) {
	return &types.User{ID: "user-a", TenantID: 10001, IsActive: true}, 10001, nil
}

type governedAdmissionMembers struct {
	interfaces.TenantMemberService
	allowed *bool
	calls   *int
}

func (m governedAdmissionMembers) GetMembership(_ context.Context, user string, tenant uint64) (*types.TenantMember, error) {
	*m.calls++
	return &types.TenantMember{UserID: user, TenantID: tenant, Status: types.TenantMemberStatusActive, OperatingAnalysisAccess: *m.allowed}, nil
}

type governedAdmissionResolver struct{}

func (governedAdmissionResolver) Resolve(context.Context, uint64) (types.GovernedEdgeConnection, error) {
	return types.GovernedEdgeConnection{EnterpriseID: "enterprise", EdgeNodeID: "edge", SourceID: "retail"}, nil
}

func (governedAdmissionResolver) VerifyCandidate(context.Context, types.GovernedEdgeBinding) error {
	return nil
}

func TestGovernedDataTurnAdmissionRejectsRevokedAndCrossTenantCredentials(t *testing.T) {
	allowed := true
	calls := 0
	h := &Handler{userService: governedAdmissionUsers{}, tenantMemberService: governedAdmissionMembers{allowed: &allowed, calls: &calls}, governedEdgeResolver: governedAdmissionResolver{}}
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user-a")
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(10001))
	ctx = types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalWebUser, ID: "user-a"})
	ctx = types.WithGovernedDataUserCredential(ctx, "private-jwt")
	agent := &types.CustomAgent{ID: "builtin-operating-analyst"}
	require.NoError(t, h.authorizeGovernedAgent(ctx, agent))
	allowed = false
	require.Error(t, h.authorizeGovernedAgent(ctx, agent))
	require.Error(t, h.authorizeGovernedAgent(context.WithValue(ctx, types.TenantIDContextKey, uint64(20002)), agent))
	require.Equal(t, 2, calls)
	require.NoError(t, h.authorizeGovernedAgent(context.Background(), &types.CustomAgent{ID: types.BuiltinEmployeeAssistantID}))
	require.Equal(t, 2, calls)
}

func TestGovernedCredentialFollowsEffectiveQAPath(t *testing.T) {
	pair := []string{tools.ToolGovernedDataSchema, tools.ToolGovernedDataQuery}
	for _, tc := range []struct {
		name, endpoint, agentID, agentMode         string
		tools                                      []string
		history, admission, credential, restricted bool
		wantMode                                   qaMode
	}{
		{"knowledge operating", "KnowledgeQA", types.BuiltinOperatingAnalystID, types.AgentModeSmartReasoning, pair, false, true, false, false, qaModeNormal},
		{"quick partial tools", "AgentQA", "custom", types.AgentModeQuickAnswer, pair[:1], false, true, false, false, qaModeNormal},
		{"quick complete tools", "AgentQA", "custom", types.AgentModeQuickAnswer, pair, false, true, false, false, qaModeNormal},
		{"agent partial tools", "AgentQA", "custom", types.AgentModeSmartReasoning, pair[:1], false, true, false, false, qaModeAgent},
		{"operating agent", "AgentQA", types.BuiltinOperatingAnalystID, types.AgentModeSmartReasoning, pair, false, true, true, true, qaModeAgent},
		{"ordinary governed history", "KnowledgeQA", "custom", types.AgentModeQuickAnswer, nil, true, false, false, true, qaModeNormal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allowed, calls := true, 0
			agent := &types.CustomAgent{ID: tc.agentID, Config: types.CustomAgentConfig{
				AgentMode: tc.agentMode, AllowedTools: tc.tools, MultiTurnEnabled: true, HistoryTurns: 3,
			}}
			history := &governedHistoryMessageService{}
			if tc.history {
				history.messages = []*types.Message{{Role: "assistant", AgentID: types.BuiltinOperatingAnalystID}}
			}
			h := &Handler{
				sessionService: &stubSessionService{}, customAgentService: &resolveOwnAgentStub{agent: agent},
				messageService: history,
				userService:    governedAdmissionUsers{}, tenantMemberService: governedAdmissionMembers{allowed: &allowed, calls: &calls}, governedEdgeResolver: governedAdmissionResolver{},
			}
			parse := func() (*qaRequestContext, error) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Params = gin.Params{{Key: "session_id", Value: "session-1"}}
				c.Set(types.UserIDContextKey.String(), "user-a")
				c.Set(types.TenantIDContextKey.String(), uint64(10001))
				ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user-a")
				ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(10001))
				ctx = types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalWebUser, ID: "user-a"})
				ctx = types.WithGovernedDataUserCredential(ctx, "private-jwt")
				c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"query":"hello","agent_id":%q}`, tc.agentID))).WithContext(ctx)
				c.Request.Header.Set("Content-Type", "application/json")
				rc, _, err := h.parseQARequest(c, tc.endpoint)
				return rc, err
			}
			rc, err := parse()
			require.NoError(t, err)
			require.Equal(t, tc.wantMode, rc.mode)
			for _, ctx := range []context.Context{rc.ctx, cloneInteractiveQATurn(rc.ctx)} {
				_, _, hasCredential := types.GovernedDataUserCredential(ctx)
				require.Equal(t, tc.credential, hasCredential)
				require.Equal(t, tc.restricted, types.GovernedDataObservability(ctx))
			}
			if tc.admission {
				require.Equal(t, 1, calls)
				allowed = false
				_, err = parse()
				require.Error(t, err, "ordinary routing must not bypass existing governed admission")
				require.Equal(t, 2, calls)
			} else {
				require.Zero(t, calls)
			}
		})
	}
}
