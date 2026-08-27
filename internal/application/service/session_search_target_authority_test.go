package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service/chat_pipeline"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type authorityKBService struct {
	interfaces.KnowledgeBaseService
	kbs                  map[string]*types.KnowledgeBase
	list                 []*types.KnowledgeBase
	listContextTenantID  uint64
	listExplicitTenantID uint64
	governedReadCalls    int
	denyGovernedRead     bool
}

func (s *authorityKBService) ListKnowledgeBasesByTenantID(ctx context.Context, tenantID uint64) ([]*types.KnowledgeBase, error) {
	s.listContextTenantID, _ = types.TenantIDFromContext(ctx)
	s.listExplicitTenantID = tenantID
	return s.list, nil
}

func (s *authorityKBService) GetKnowledgeBasesByIDsOnly(_ context.Context, ids []string) ([]*types.KnowledgeBase, error) {
	out := make([]*types.KnowledgeBase, 0, len(ids))
	for _, id := range ids {
		if kb := s.kbs[id]; kb != nil {
			out = append(out, kb)
		}
	}
	return out, nil
}

func (s *authorityKBService) GetKnowledgeBaseByID(_ context.Context, id string) (*types.KnowledgeBase, error) {
	s.governedReadCalls++
	if s.denyGovernedRead {
		return nil, errors.New("governance denied")
	}
	if kb := s.kbs[id]; kb != nil {
		return kb, nil
	}
	return nil, errors.New("governance denied")
}

func (s *authorityKBService) GetKnowledgeBaseByIDOnly(_ context.Context, id string) (*types.KnowledgeBase, error) {
	if kb := s.kbs[id]; kb != nil {
		return kb, nil
	}
	return nil, errors.New("missing")
}

type authorityKnowledgeService struct {
	interfaces.KnowledgeService
	knowledges          map[string]*types.Knowledge
	batchContextTenant  uint64
	batchExplicitTenant uint64
	batchCalls          int
	sharedBatchCalls    int
	tagCalls            int
	ignoreTenantFilter  bool
}

func (s *authorityKnowledgeService) GetKnowledgeBatch(ctx context.Context, tenantID uint64, ids []string) ([]*types.Knowledge, error) {
	s.batchCalls++
	s.batchContextTenant, _ = types.TenantIDFromContext(ctx)
	s.batchExplicitTenant = tenantID
	out := make([]*types.Knowledge, 0, len(ids))
	for _, id := range ids {
		if k := s.knowledges[id]; k != nil && (s.ignoreTenantFilter || k.TenantID == tenantID) {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *authorityKnowledgeService) GetKnowledgeBatchWithSharedAccess(_ context.Context, _ uint64, ids []string) ([]*types.Knowledge, error) {
	s.sharedBatchCalls++
	out := make([]*types.Knowledge, 0, len(ids))
	for _, id := range ids {
		if k := s.knowledges[id]; k != nil {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *authorityKnowledgeService) ListKnowledgeIDsByTagIDs(_ context.Context, _ uint64, _ string, _ []string) ([]string, error) {
	s.tagCalls++
	return nil, nil
}

type authorityKBShareService struct {
	interfaces.KBShareService
	permissionCalls int
	allow           bool
}

func (s *authorityKBShareService) HasTenantKBPermission(
	context.Context, string, uint64, types.TenantRole, types.OrgMemberRole,
) (bool, error) {
	s.permissionCalls++
	return s.allow, nil
}

func sharedAuthorityContext() context.Context {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "human")
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(20))
	ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleAdmin)
	return types.WithAuthorizedSharedAgentExecution(ctx, 10, 20, "agent-a")
}

func sharedAuthorityRequest(mode string, configured ...string) *types.QARequest {
	return &types.QARequest{
		Session:             &types.Session{ID: "session-a", TenantID: 10},
		SharedAgentReadOnly: true,
		CustomAgent: &types.CustomAgent{
			ID:       "agent-a",
			TenantID: 20,
			Config: types.CustomAgentConfig{
				KBSelectionMode: mode,
				KnowledgeBases:  configured,
			},
		},
	}
}

func TestBuildSharedAgentSearchScope_AllUsesCallerContextAndExplicitSource(t *testing.T) {
	kbs := &authorityKBService{list: []*types.KnowledgeBase{
		{ID: "kb-a", TenantID: 20, IndexingStrategy: types.IndexingStrategy{VectorEnabled: true}},
		{ID: "kb-incompatible", TenantID: 20, IndexingStrategy: types.IndexingStrategy{WikiEnabled: true}},
	}}
	svc := &sessionService{knowledgeBaseService: kbs}
	req := sharedAuthorityRequest("all")
	req.CustomAgent.Config.AllowedTools = []string{"knowledge_search"}

	scope, err := svc.buildSharedAgentSearchScope(sharedAuthorityContext(), req)

	require.NoError(t, err)
	require.Equal(t, uint64(10), kbs.listContextTenantID)
	require.Equal(t, uint64(20), kbs.listExplicitTenantID)
	require.Equal(t, []string{"kb-a"}, scope.allowedKBIDs)
	resolvedKBIDs, _, err := svc.resolveKnowledgeBases(sharedAuthorityContext(), req, scope)
	require.NoError(t, err)
	require.Equal(t, []string{"kb-a"}, resolvedKBIDs)
}

func TestBuildSharedAgentSearchScope_SelectionTruthTableAndSourceOwnership(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       string
		configured []string
		list       []*types.KnowledgeBase
		want       []string
		wantErr    bool
	}{
		{name: "selected", mode: "selected", configured: []string{"kb-a"}, want: []string{"kb-a"}},
		{name: "default is selected", configured: []string{"kb-a"}, want: []string{"kb-a"}},
		{name: "none", mode: "none", configured: []string{"kb-a"}, want: nil},
		{name: "all", mode: "all", list: []*types.KnowledgeBase{{ID: "kb-a", TenantID: 20}}, want: []string{"kb-a"}},
		{name: "all rejects third tenant row", mode: "all", list: []*types.KnowledgeBase{{ID: "kb-third", TenantID: 30}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &sessionService{knowledgeBaseService: &authorityKBService{list: tc.list}}
			scope, err := svc.buildSharedAgentSearchScope(sharedAuthorityContext(), sharedAuthorityRequest(tc.mode, tc.configured...))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, scope.allowedKBIDs)
		})
	}
}

func TestBuildSearchTargets_SharedAgentUsesExactAllowedSourceKBWithoutGovernance(t *testing.T) {
	kbs := &authorityKBService{kbs: map[string]*types.KnowledgeBase{
		"kb-a":     {ID: "kb-a", TenantID: 20},
		"kb-other": {ID: "kb-other", TenantID: 20},
		"kb-third": {ID: "kb-third", TenantID: 30},
	}}
	svc := &sessionService{knowledgeBaseService: kbs, knowledgeService: &authorityKnowledgeService{}}
	scope := &sharedAgentSearchScope{callerTenantID: 10, sourceTenantID: 20, agentID: "agent-a", allowedKBIDs: []string{"kb-a", "kb-third"}}

	targets, err := svc.buildSearchTargets(sharedAuthorityContext(), 20, []string{"kb-a"}, nil, nil, scope)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, uint64(20), targets[0].TenantID)
	require.Zero(t, kbs.governedReadCalls, "shared-Agent scope must not require ordinary source governance")

	_, err = svc.buildSearchTargets(sharedAuthorityContext(), 20, []string{"kb-other"}, nil, nil, scope)
	require.Error(t, err, "an arbitrary source KB is outside the exact Agent set")
	_, err = svc.buildSearchTargets(sharedAuthorityContext(), 20, []string{"kb-third"}, nil, nil, scope)
	require.Error(t, err, "an allowed ID must still resolve to a source-owned KB")
}

func TestBuildSearchTargets_SharedDocumentMustBelongToAllowedSourceKB(t *testing.T) {
	kbs := &authorityKBService{kbs: map[string]*types.KnowledgeBase{
		"kb-a":     {ID: "kb-a", TenantID: 20},
		"kb-other": {ID: "kb-other", TenantID: 20},
		"kb-third": {ID: "kb-third", TenantID: 30},
	}}
	knowledge := &authorityKnowledgeService{
		ignoreTenantFilter: true,
		knowledges: map[string]*types.Knowledge{
			"doc-ok":        {ID: "doc-ok", TenantID: 20, KnowledgeBaseID: "kb-a"},
			"doc-arbitrary": {ID: "doc-arbitrary", TenantID: 20, KnowledgeBaseID: "kb-other"},
			"doc-third":     {ID: "doc-third", TenantID: 30, KnowledgeBaseID: "kb-third"},
		},
	}
	svc := &sessionService{knowledgeBaseService: kbs, knowledgeService: knowledge}
	scope := &sharedAgentSearchScope{callerTenantID: 10, sourceTenantID: 20, agentID: "agent-a", allowedKBIDs: []string{"kb-a", "kb-third"}}

	targets, err := svc.buildSearchTargets(sharedAuthorityContext(), 20, nil, []string{"doc-ok"}, nil, scope)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, []string{"doc-ok"}, targets[0].KnowledgeIDs)
	require.Equal(t, uint64(10), knowledge.batchContextTenant)
	require.Equal(t, uint64(20), knowledge.batchExplicitTenant)
	require.Zero(t, knowledge.sharedBatchCalls)

	_, err = svc.buildSearchTargets(sharedAuthorityContext(), 20, nil, []string{"doc-arbitrary"}, nil, scope)
	require.Error(t, err)
	_, err = svc.buildSearchTargets(sharedAuthorityContext(), 20, nil, []string{"doc-third"}, nil, scope)
	require.Error(t, err)
}

func apiKeyAuthorityContext() context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(10))
	return types.WithTenantAPIKeyScope(ctx, types.TenantAPIKeyScope{
		KnowledgeBaseIDs: types.StringArray{"own"},
	})
}

func TestBuildSearchTargets_APIKeyRejectsForeignKBTagAndDocumentWithoutShareProbes(t *testing.T) {
	kbs := &authorityKBService{kbs: map[string]*types.KnowledgeBase{
		"own":     {ID: "own", TenantID: 10, Type: types.KnowledgeBaseTypeDocument},
		"foreign": {ID: "foreign", TenantID: 20, Type: types.KnowledgeBaseTypeDocument},
	}}
	knowledge := &authorityKnowledgeService{knowledges: map[string]*types.Knowledge{
		"own-doc":     {ID: "own-doc", TenantID: 10, KnowledgeBaseID: "own"},
		"foreign-doc": {ID: "foreign-doc", TenantID: 20, KnowledgeBaseID: "foreign"},
	}}
	shares := &authorityKBShareService{allow: true}
	svc := &sessionService{knowledgeBaseService: kbs, knowledgeService: knowledge, kbShareService: shares}

	for _, tc := range []struct {
		name string
		kbs  []string
		docs []string
		tags []types.TagScope
	}{
		{name: "foreign KB", kbs: []string{"foreign"}},
		{name: "foreign tag", tags: []types.TagScope{{KnowledgeBaseID: "foreign", TagIDs: []string{"tag"}}}},
		{name: "foreign document", docs: []string{"foreign-doc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.buildSearchTargets(apiKeyAuthorityContext(), 10, tc.kbs, tc.docs, tc.tags, nil)
			require.Error(t, err)
		})
	}
	require.Zero(t, shares.permissionCalls)
	require.Zero(t, knowledge.sharedBatchCalls)
	require.Zero(t, knowledge.tagCalls, "foreign tag must fail before durable tag resolution")

	targets, err := svc.buildSearchTargets(apiKeyAuthorityContext(), 10, []string{"own"}, []string{"own-doc"}, nil, nil)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Zero(t, shares.permissionCalls)
}

type pipelineGovernanceProbe struct {
	events []types.EventType
	kbs    *authorityKBService
}

func (p *pipelineGovernanceProbe) ActivationEvents() []types.EventType {
	return []types.EventType{types.CHUNK_SEARCH, types.CHAT_COMPLETION_STREAM}
}

func (p *pipelineGovernanceProbe) OnEvent(
	_ context.Context, eventType types.EventType, _ *types.ChatManage, next func() *chatpipeline.PluginError,
) *chatpipeline.PluginError {
	p.events = append(p.events, eventType)
	if eventType == types.CHUNK_SEARCH {
		p.kbs.denyGovernedRead = true
	}
	return next()
}

func TestKnowledgeQAByEvent_RechecksSameTenantGovernanceBeforeFinalOutput(t *testing.T) {
	kbs := &authorityKBService{kbs: map[string]*types.KnowledgeBase{
		"kb-a": {ID: "kb-a", TenantID: 20},
	}}
	manager := chatpipeline.NewEventManager()
	probe := &pipelineGovernanceProbe{kbs: kbs}
	manager.Register(probe)
	svc := &sessionService{eventManager: manager, knowledgeBaseService: kbs}
	chatManage := &types.ChatManage{PipelineRequest: types.PipelineRequest{
		SessionID: "session-a",
		SearchTargets: types.SearchTargets{{
			Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb-a", TenantID: 20,
		}},
	}}
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "employee")
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(20))
	ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleViewer)

	err := svc.KnowledgeQAByEvent(
		ctx, chatManage, []types.EventType{types.CHUNK_SEARCH, types.CHAT_COMPLETION_STREAM},
	)

	require.Error(t, err)
	require.Equal(t, []types.EventType{types.CHUNK_SEARCH}, probe.events)
}
