package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmployeeMissingUserConditionCanStillTriggerRetrieval(t *testing.T) {
	for _, web := range []bool{false, true} {
		need := true
		cm := &ChatManage{PipelineRequest: PipelineRequest{EmployeeAssistant: true, WebSearchEnabled: web}, PipelineState: PipelineState{Intent: IntentNeedsUserInput, RetrievalNeeded: &need}}
		require.True(t, cm.NeedsRetrieval())
	}
	legacy := &ChatManage{PipelineState: PipelineState{Intent: IntentNeedsUserInput}}
	require.False(t, legacy.NeedsRetrieval(), "generic agents preserve intent routing")
	require.True(t, IntentKBSearch.NeedsKBRetrieval())
	require.True(t, IntentClarification.NeedsKBRetrieval()) // historical agents unchanged
}

func TestEmployeeAbsentRetrievalDecisionDoesNotSuppressFreshEvidence(t *testing.T) {
	for _, intent := range []QueryIntent{IntentNeedsUserInput, IntentFollowUp} {
		cm := &ChatManage{PipelineRequest: PipelineRequest{EmployeeAssistant: true}, PipelineState: PipelineState{Intent: intent}}
		require.Truef(t, cm.NeedsRetrieval(), "intent %q", intent)
	}
}

func TestChatManageCloneCarriesEmployeeEvidenceDecision(t *testing.T) {
	need := true
	original := &ChatManage{
		PipelineRequest: PipelineRequest{EmployeeAssistant: true},
		PipelineState:   PipelineState{RetrievalNeeded: &need, MissingUserCondition: "device model", RetrievalExecuted: true, RetrievalDegraded: true, RerankExecuted: true, RerankFailed: true},
	}
	clone := original.Clone()
	require.True(t, clone.EmployeeAssistant)
	require.NotSame(t, original.RetrievalNeeded, clone.RetrievalNeeded)
	require.True(t, *clone.RetrievalNeeded)
	require.Equal(t, "device model", clone.MissingUserCondition)
	require.True(t, clone.RetrievalExecuted)
	require.True(t, clone.RetrievalDegraded)
	require.True(t, clone.RerankExecuted)
	require.True(t, clone.RerankFailed)
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
