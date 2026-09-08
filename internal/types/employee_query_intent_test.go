package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingUserConditionDoesNotTriggerRetrieval(t *testing.T) {
	for _, web := range []bool{false, true} {
		cm := &ChatManage{PipelineRequest: PipelineRequest{WebSearchEnabled: web}, PipelineState: PipelineState{Intent: IntentNeedsUserInput}}
		require.False(t, cm.NeedsRetrieval())
	}
	require.True(t, IntentKBSearch.NeedsKBRetrieval())
	require.True(t, IntentClarification.NeedsKBRetrieval()) // historical agents unchanged
}

func TestQueryIntentUnknownDefaultsToRetrieval(t *testing.T) {
	require.True(t, QueryIntent("").NeedsKBRetrieval())
	require.True(t, QueryIntent("future_intent").NeedsKBRetrieval())
}

func TestChatManageNeedsRetrievalForKnownIntents(t *testing.T) {
	for _, intent := range []QueryIntent{
		IntentKBSearch,
		IntentClarification,
		IntentSummarize,
	} {
		cm := &ChatManage{PipelineState: PipelineState{Intent: intent}}
		require.Truef(t, cm.NeedsRetrieval(), "intent %q", intent)
	}

	for _, intent := range []QueryIntent{
		IntentGreeting,
		IntentChitchat,
		IntentFollowUp,
		IntentImageOnly,
		IntentDocOnly,
		IntentNeedsUserInput,
	} {
		cm := &ChatManage{PipelineState: PipelineState{Intent: intent}}
		require.Falsef(t, cm.NeedsRetrieval(), "intent %q", intent)
	}

	for _, enabled := range []bool{false, true} {
		cm := &ChatManage{
			PipelineRequest: PipelineRequest{WebSearchEnabled: enabled},
			PipelineState:   PipelineState{Intent: IntentWebSearch},
		}
		require.Equalf(t, enabled, cm.NeedsRetrieval(), "web search enabled=%v", enabled)
	}
}
