package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveChatModelIDUsesPlatformDefaultWhenAgentBindingIsEmpty(t *testing.T) {
	svc := &sessionService{
		modelService: &stubModelService{
			modelsByID: map[string]*types.Model{
				"builtin-chat": {
					ID:     "builtin-chat",
					Type:   types.ModelTypeKnowledgeQA,
					Status: types.ModelStatusActive,
				},
			},
		},
	}
	req := &types.QARequest{
		Session: &types.Session{},
		CustomAgent: &types.CustomAgent{
			ID: "agent-1",
		},
		SummaryModelID: "builtin-chat",
	}

	modelID, err := svc.resolveChatModelID(context.Background(), req, nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "builtin-chat", modelID)
}

func TestResolveChatModelIDFallsBackWhenAgentBindingIsUnavailable(t *testing.T) {
	svc := &sessionService{
		modelService: &stubModelService{modelsByID: map[string]*types.Model{
			"platform-default": {ID: "platform-default", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
		}},
	}
	req := &types.QARequest{
		Session: &types.Session{},
		CustomAgent: &types.CustomAgent{
			ID: "agent-1",
			Config: types.CustomAgentConfig{
				ModelID: "deleted-model",
			},
		},
	}

	modelID, err := svc.resolveChatModelID(context.Background(), req, nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "platform-default", modelID)
}

func TestResolveChatModelIDUsesValidConfiguredAgentModel(t *testing.T) {
	svc := &sessionService{
		modelService: &stubModelService{
			modelsByID: map[string]*types.Model{
				"agent-chat": {
					ID:   "agent-chat",
					Type: types.ModelTypeKnowledgeQA,
				},
			},
		},
	}
	req := &types.QARequest{
		Session: &types.Session{},
		CustomAgent: &types.CustomAgent{
			ID: "agent-1",
			Config: types.CustomAgentConfig{
				ModelID: "agent-chat",
			},
		},
	}

	modelID, err := svc.resolveChatModelID(context.Background(), req, nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "agent-chat", modelID)
}

func TestResolveChatModelIDRejectsNonChatSummaryModelOverride(t *testing.T) {
	svc := &sessionService{
		modelService: &stubModelService{
			modelsByID: map[string]*types.Model{
				"agent-chat": {
					ID:   "agent-chat",
					Type: types.ModelTypeKnowledgeQA,
				},
				"rerank-only": {
					ID:   "rerank-only",
					Type: types.ModelTypeRerank,
				},
			},
		},
	}
	req := &types.QARequest{
		Session: &types.Session{},
		CustomAgent: &types.CustomAgent{
			ID: "agent-1",
			Config: types.CustomAgentConfig{
				ModelID: "agent-chat",
			},
		},
		SummaryModelID: "rerank-only",
	}

	modelID, err := svc.resolveChatModelID(context.Background(), req, nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "agent-chat", modelID)
}

func TestResolveChatModelIDIgnoresRequestModelForAgent(t *testing.T) {
	svc := &sessionService{
		modelService: &stubModelService{
			modelsByID: map[string]*types.Model{
				"agent-chat": {
					ID:   "agent-chat",
					Type: types.ModelTypeKnowledgeQA,
				},
				"override-chat": {
					ID:   "override-chat",
					Type: types.ModelTypeKnowledgeQA,
				},
			},
		},
	}
	req := &types.QARequest{
		Session: &types.Session{},
		CustomAgent: &types.CustomAgent{
			ID: "agent-1",
			Config: types.CustomAgentConfig{
				ModelID: "agent-chat",
			},
		},
		SummaryModelID: "override-chat",
	}

	modelID, err := svc.resolveChatModelID(context.Background(), req, nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "agent-chat", modelID)
}

func TestResolveChatModelIDWikiFixerFallsBackToKnowledgeBaseModel(t *testing.T) {
	svc := &sessionService{
		modelService: &stubModelService{
			modelsByID: map[string]*types.Model{
				"wiki-chat": {
					ID:   "wiki-chat",
					Type: types.ModelTypeKnowledgeQA,
				},
			},
		},
		knowledgeBaseService: &fakeAgentKnowledgeBaseService{
			kb: &types.KnowledgeBase{
				ID:             "wiki-kb",
				SummaryModelID: "wiki-chat",
			},
		},
	}
	req := &types.QARequest{
		Session: &types.Session{},
		CustomAgent: &types.CustomAgent{
			ID: types.BuiltinWikiFixerID,
		},
	}

	modelID, err := svc.resolveChatModelID(context.Background(), req, []string{"wiki-kb"}, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "wiki-chat", modelID)
}

func TestResolveChatModelIDWikiFixerFallsBackToAvailableModel(t *testing.T) {
	svc := &sessionService{
		modelService: &stubModelService{
			availableModels: []*types.Model{
				{
					ID:     "system-chat",
					Type:   types.ModelTypeKnowledgeQA,
					Status: types.ModelStatusActive,
				},
			},
		},
	}
	req := &types.QARequest{
		Session: &types.Session{},
		CustomAgent: &types.CustomAgent{
			ID: types.BuiltinWikiFixerID,
		},
	}

	modelID, err := svc.resolveChatModelID(context.Background(), req, nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "system-chat", modelID)
}

func TestResolveChatModelIDFallbackUsesActiveDefault(t *testing.T) {
	alternative := &types.Model{ID: "alternative", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive}
	preferred := &types.Model{ID: "preferred", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true}
	disabled := &types.Model{ID: "disabled-default", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusDownloadFailed, IsDefault: true}
	for _, tc := range []struct {
		name   string
		models []*types.Model
		want   string
	}{
		{"default after alternative", []*types.Model{alternative, preferred}, "preferred"},
		{"default before alternative", []*types.Model{preferred, alternative}, "preferred"},
		{"disabled default skipped", []*types.Model{disabled, alternative}, "alternative"},
		{"default absent", []*types.Model{alternative}, "alternative"},
		{"no active model", []*types.Model{disabled}, ""},
		{"empty list", []*types.Model{}, ""},
		{"other type ignored", []*types.Model{nil, {ID: "embedding", Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive, IsDefault: true}, alternative}, "alternative"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &sessionService{modelService: &stubModelService{availableModels: tc.models}}
			req := &types.QARequest{Session: &types.Session{}, CustomAgent: &types.CustomAgent{ID: types.BuiltinOperatingAnalystID}}
			id, err := svc.resolveChatModelID(context.Background(), req, nil, nil, nil)
			if tc.want == "" {
				require.ErrorContains(t, err, "no available models")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, id)
		})
	}
}

func TestResolveChatModelIDKeepsBindingAndKnowledgeBasePriority(t *testing.T) {
	for _, configured := range []bool{true, false} {
		name := "knowledge base"
		if configured {
			name = "explicit binding"
		}
		t.Run(name, func(t *testing.T) {
			svc := &sessionService{
				modelService: &stubModelService{
					modelsByID:      map[string]*types.Model{"bound": {ID: "bound", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive}},
					availableModels: []*types.Model{{ID: "default", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true}},
				},
				knowledgeBaseService: &fakeAgentKnowledgeBaseService{kb: &types.KnowledgeBase{ID: "kb", SummaryModelID: "bound"}},
			}
			req := &types.QARequest{Session: &types.Session{}, CustomAgent: &types.CustomAgent{ID: "agent"}}
			var kbIDs []string
			if configured {
				req.CustomAgent.Config.ModelID = "bound"
			} else {
				kbIDs = []string{"kb"}
			}
			id, err := svc.resolveChatModelID(context.Background(), req, kbIDs, nil, nil)
			require.NoError(t, err)
			require.Equal(t, "bound", id)
		})
	}
}
