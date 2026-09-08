package chunker

import (
	"strings"
	"unicode/utf8"
)

// WithDocumentFormat applies the parser's structural contract, not a guess
// based on the file name or the text's business vocabulary.
func WithDocumentFormat(cfg SplitterConfig, metadata map[string]string) SplitterConfig {
	cfg.RecordLines = metadata["content_format"] == "spreadsheet_rows_v1"
	return cfg
}

func splitRecordLines(text string, cfg SplitterConfig) []Chunk {
	cfg.RecordLines = false
	var chunks []Chunk
	offset := 0
	for _, record := range strings.SplitAfter(text, "\n") {
		if strings.TrimSpace(record) != "" {
			for _, part := range Split(record, cfg) {
				part.Seq = len(chunks)
				part.Start += offset
				part.End += offset
				chunks = append(chunks, part)
			}
		}
		offset += utf8.RuneCountInString(record)
	}
	return chunks
}
