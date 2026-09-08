package types

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMissingUserConditionDoesNotTriggerRetrieval(t *testing.T) {
	for _, web := range []bool{false, true} {
		cm := &ChatManage{PipelineRequest: PipelineRequest{WebSearchEnabled: web}, PipelineState: PipelineState{Intent: IntentNeedsUserInput}}
		require.False(t, cm.NeedsRetrieval())
	}
	require.True(t, IntentKBSearch.NeedsKBRetrieval())
	require.True(t, IntentClarification.NeedsKBRetrieval()) // historical agents unchanged
}
