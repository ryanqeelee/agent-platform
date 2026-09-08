package agent

import (
	"strings"
	"testing"
)

func TestFinalAnswerImageRequirement(t *testing.T) {
	if got := finalAnswerImageRequirement(false); got != "" {
		t.Fatalf("text-only tool results should not add an image requirement: %q", got)
	}

	got := finalAnswerImageRequirement(true)
	for _, required := range []string{
		"only when it directly supports the user's requested answer",
		"Preserve its complete URL exactly",
		"ASCII half-width parentheses",
		"silently verify",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("expected %q in final-answer image requirement:\n%s", required, got)
		}
	}
}
