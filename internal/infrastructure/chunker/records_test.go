package chunker

import (
	"strings"
	"testing"
)

func TestSpreadsheetRecordsStaySeparate(t *testing.T) {
	text := "[工作表: 客服; 行: 1] 问题: 甲🙂,答: 插好USB\n[工作表: 客服; 行: 3] 问题: 乙,答: 校准日期\n"
	cfg := WithDocumentFormat(DefaultConfig(), map[string]string{"content_format": "spreadsheet_rows_v1"})
	parts := Split(text, cfg)
	if len(parts) != 2 {
		t.Fatalf("records merged: %#v", parts)
	}
	for _, p := range parts {
		if string([]rune(text)[p.Start:p.End]) != p.Content {
			t.Fatal("offset mismatch")
		}
		if strings.Contains(p.Content, "甲") && strings.Contains(p.Content, "乙") {
			t.Fatal("cross-record content")
		}
	}
	parent, child := DeriveParentChildConfigs(cfg, 4096, 384)
	pc := SplitParentChild(text, parent, child)
	if len(pc.Children) != 2 {
		t.Fatalf("parent-child merged records: %#v", pc)
	}
	for _, p := range pc.Children {
		if strings.Contains(p.Content, "甲") && strings.Contains(p.Content, "乙") {
			t.Fatal("parent-child crossed record")
		}
	}
}

func TestSpreadsheetLongRecordAndBlankLines(t *testing.T) {
	first := strings.Repeat("中文内容。", 300) + "\n"
	text := first + "\n第二条🙂\n"
	cfg := WithDocumentFormat(DefaultConfig(), map[string]string{"content_format": "spreadsheet_rows_v1"})
	parts := Split(text, cfg)
	if len(parts) < 3 {
		t.Fatal("oversized record was not split")
	}
	covered := make([]bool, len([]rune(text)))
	for _, p := range parts {
		if string([]rune(text)[p.Start:p.End]) != p.Content {
			t.Fatal("offset mismatch")
		}
		if strings.Contains(p.Content, "中文") && strings.Contains(p.Content, "第二") {
			t.Fatal("cross-record overlap")
		}
		for i := p.Start; i < p.End; i++ {
			covered[i] = true
		}
	}
	for i, r := range []rune(text) {
		if r != '\n' && !covered[i] {
			t.Fatalf("lost content at %d", i)
		}
	}
	for _, format := range []string{"", "unknown"} {
		if WithDocumentFormat(DefaultConfig(), map[string]string{"content_format": format}).RecordLines {
			t.Fatal("unknown format enabled")
		}
	}
}
