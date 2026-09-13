package session

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
)

type attachmentParserConfigRepo struct {
	config *types.ParserEngineConfig
}

func (r attachmentParserConfigRepo) Get(context.Context) (*types.PlatformParserConfig, error) {
	return &types.PlatformParserConfig{ID: types.PlatformParserConfigSingletonID, Config: r.config}, nil
}

func (attachmentParserConfigRepo) Upsert(
	context.Context, *types.ParserEngineConfig, string, time.Time,
) (*types.PlatformParserConfig, error) {
	panic("unexpected parser config write")
}

type attachmentDocumentReader struct {
	readCalls int
}

func (r *attachmentDocumentReader) Read(context.Context, *types.ReadRequest) (*types.ReadResult, error) {
	r.readCalls++
	return &types.ReadResult{MarkdownContent: "remote content"}, nil
}

func (*attachmentDocumentReader) Reconnect(string) error { return nil }
func (*attachmentDocumentReader) IsConnected() bool      { return true }
func (*attachmentDocumentReader) ListEngines(context.Context, map[string]string) ([]types.ParserEngineInfo, error) {
	return nil, nil
}

func TestAttachmentProcessorSelectedEngineConstructionFailureDoesNotFallback(t *testing.T) {
	remote := &attachmentDocumentReader{}
	processor := NewAttachmentProcessor(
		nil, remote, nil, nil,
		service.NewPlatformParserConfigService(attachmentParserConfigRepo{config: &types.ParserEngineConfig{
			ChatParserEngineRules: []types.ParserEngineRule{{FileTypes: []string{"pdf"}, Engine: "weknoracloud"}},
		}}),
	)

	err := processor.processWithDocumentReader(
		context.Background(), []byte("pdf"), "attachment.pdf", ".pdf", &types.MessageAttachment{}, 41,
	)
	if err == nil {
		t.Fatal("selected retired parser succeeded, want explicit construction failure")
	}
	if remote.readCalls != 0 {
		t.Fatalf("selected parser construction failure fell back to DocReader: read calls = %d", remote.readCalls)
	}
}

func TestAttachmentProcessorAutomaticRoutingStillUsesDocReader(t *testing.T) {
	remote := &attachmentDocumentReader{}
	processor := NewAttachmentProcessor(
		nil, remote, nil, nil,
		service.NewPlatformParserConfigService(attachmentParserConfigRepo{config: &types.ParserEngineConfig{}}),
	)
	attachment := &types.MessageAttachment{}

	err := processor.processWithDocumentReader(
		context.Background(), []byte("custom"), "attachment.custom", ".custom", attachment, 41,
	)
	if err != nil {
		t.Fatalf("automatic DocReader routing failed: %v", err)
	}
	if remote.readCalls != 1 {
		t.Fatalf("automatic routing read calls = %d, want 1", remote.readCalls)
	}
	if attachment.Content != "remote content" {
		t.Fatalf("automatic routing content = %q, want remote content", attachment.Content)
	}
}
