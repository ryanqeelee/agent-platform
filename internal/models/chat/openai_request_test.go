package chat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChatCompletionRequestSerializesNoneToolChoiceWithoutTools(t *testing.T) {
	client := newTestRemoteChat(t)
	req := client.BuildChatCompletionRequest(
		[]Message{{Role: "user", Content: "answer only"}},
		&ChatOptions{ToolChoice: "none"},
		true,
	)

	assert.Empty(t, req.Tools)
	assert.Equal(t, "none", req.ToolChoice)
	body, err := json.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, `"none"`, string(bodyField(t, body, "tool_choice")))
	assert.NotContains(t, string(body), `"tools"`)
}

func bodyField(t *testing.T, body []byte, field string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields))
	value, ok := fields[field]
	require.True(t, ok, "missing %q in request JSON: %s", field, body)
	return value
}
