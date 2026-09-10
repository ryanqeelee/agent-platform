package service

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestGovernedAgentQAStartLogIsMetadataOnly(t *testing.T) {
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	logger.SetLogLevel(logger.LevelInfo)
	t.Cleanup(func() {
		logger.SetOutput(os.Stdout)
		logger.SetLogLevel(logger.LevelDebug)
	})

	ctx := types.WithGovernedDataObservability(context.Background())
	logAgentQAStart(ctx, "session-1", 10001, "QUERY_SENTINEL", []byte(`{"history":"ROWS_SENTINEL","credential":"CREDENTIAL_SENTINEL"}`))
	got := logs.String()
	for _, secret := range []string{"QUERY_SENTINEL", "ROWS_SENTINEL", "CREDENTIAL_SENTINEL"} {
		if strings.Contains(got, secret) {
			t.Fatalf("governed AgentQA start log leaked %q: %s", secret, got)
		}
	}
	for _, metadata := range []string{"session-1", "10001", "query_bytes", "session_payload_omitted=true"} {
		if !strings.Contains(got, metadata) {
			t.Fatalf("governed AgentQA start log lost %q: %s", metadata, got)
		}
	}
}

func TestGovernedAgentImageDescriptionPreservesDraftInstruction(t *testing.T) {
	query := "分析经营数据\n\n" + tools.GovernedAnalysisDraftInstruction
	got := appendAgentImageDescription(query, "图中显示华东门店")
	if !strings.Contains(got, tools.GovernedAnalysisDraftInstruction) {
		t.Fatal("image description discarded governed draft instruction")
	}
	if !strings.Contains(got, "[用户上传图片内容]\n图中显示华东门店") {
		t.Fatal("image description was not appended")
	}
}

func TestOrdinaryAgentQAStartLogKeepsPayload(t *testing.T) {
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	logger.SetLogLevel(logger.LevelInfo)
	t.Cleanup(func() {
		logger.SetOutput(os.Stdout)
		logger.SetLogLevel(logger.LevelDebug)
	})

	logAgentQAStart(context.Background(), "session-1", 10001, "ordinary-query", []byte(`{"history":"ordinary-history"}`))
	got := logs.String()
	for _, want := range []string{"ordinary-query", "ordinary-history"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ordinary AgentQA start log lost %q: %s", want, got)
		}
	}
}
