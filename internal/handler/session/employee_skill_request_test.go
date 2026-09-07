package session

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmployeeSkillSelectionReachesNormalRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		foreign uint64
		mcp     []string
		want    string
	}{
		{name: "enterprise skill selection", want: "resource_urls"},
		{name: "foreign agent authority", foreign: 84, want: "基础能力"},
		{name: "request supplied mcp", mcp: []string{"outside"}, want: "基础能力"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(CreateKnowledgeQARequest{
				Query: "use selected skill", AgentID: types.BuiltinEmployeeAssistantID,
				SkillNames: []string{"数据处理器"}, AgentSourceTenantID: tc.foreign, MCPServiceIDs: tc.mcp,
			})
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Params = gin.Params{{Key: "session_id", Value: "session-1"}}
			// A later validation failure proves the request was not rejected by
			// the obsolete skill ban, without invoking an agent or remote provider.
			c.Request = httptest.NewRequest(http.MethodPost, "/agent-chat/session-1?resource_urls=signed", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			_, _, err = (&Handler{}).parseQARequest(c, "AgentQA")
			require.ErrorContains(t, err, tc.want)
		})
	}
}
