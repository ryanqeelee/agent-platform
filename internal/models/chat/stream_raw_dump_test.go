package chat

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestGovernedStreamRawDumpStaysDisabledWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEKNORA_LLM_STREAM_RAW_DUMP_DIR", dir)
	ctx := types.WithGovernedDataObservability(context.Background())
	if dumper := newStreamPacketDumper(ctx, "model", map[string]string{"query": "QUERY_SENTINEL"}); dumper != nil {
		dumper.Close()
		t.Fatal("governed turn created a raw stream dumper")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("governed turn wrote %d raw dump files", len(entries))
	}
}

func TestOrdinaryStreamRawDumpStillWritesConfiguredPayload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WEKNORA_LLM_STREAM_RAW_DUMP_DIR", dir)
	dumper := newStreamPacketDumper(context.Background(), "model", map[string]string{"query": "ordinary-query"})
	if dumper == nil {
		t.Fatal("ordinary turn did not create configured raw stream dumper")
	}
	path := dumper.Path()
	dumper.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ordinary-query") {
		t.Fatalf("ordinary raw dump lost request payload: %s", data)
	}
}
