package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func strictMemoryContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/memory/commands", strings.NewReader(body))
	return c
}

func TestMemoryCommandDecoderRejectsOwnerFieldsAndTrailingJSON(t *testing.T) {
	valid := `{"schema":"personal_memory_command/1","operation_id":"op","source":{"runtime":"employee","mode":"manual","session_id":"s","message_id":"m"},"expected":{"workspace_generation":0,"subject_generation":0,"revision":0},"changes":[{"op":"create","kind":"fact","content":"x"}]}`
	var command types.PersonalMemoryCommand
	require.NoError(t, decodeStrictMemoryJSON(strictMemoryContext(t, valid), &command))

	withOwner := strings.TrimSuffix(valid, "}") + `,"tenant_id":99}`
	require.ErrorContains(t,
		decodeStrictMemoryJSON(strictMemoryContext(t, withOwner), &types.PersonalMemoryCommand{}),
		"unknown field",
	)
	require.Error(t,
		decodeStrictMemoryJSON(strictMemoryContext(t, valid+` {}`), &types.PersonalMemoryCommand{}),
	)
}

func TestMemoryCommandDecoderKeepsIntegerFieldsStrict(t *testing.T) {
	body := `{"schema":"personal_memory_command/1","operation_id":"op","source":{"runtime":"employee","mode":"manual","session_id":"s","message_id":"m"},"expected":{"workspace_generation":0,"subject_generation":0,"revision":0.5},"changes":[{"op":"create","kind":"fact","content":"x"}]}`
	require.Error(t,
		decodeStrictMemoryJSON(strictMemoryContext(t, body), &types.PersonalMemoryCommand{}),
	)
	missing := `{"schema":"personal_memory_command/1","operation_id":"op","source":{"runtime":"employee","mode":"manual","session_id":"s","message_id":"m"},"expected":{},"changes":[{"op":"create","kind":"fact","content":"x"}]}`
	require.Error(t,
		decodeStrictMemoryJSON(strictMemoryContext(t, missing), &types.PersonalMemoryCommand{}),
	)
}
