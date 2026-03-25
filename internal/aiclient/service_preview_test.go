package aiclient

import (
	"strings"
	"testing"
)

func TestPreviewBodyTruncatesOnRuneBoundary(t *testing.T) {
	input := []byte(strings.Repeat("測", 600))

	preview := previewBody(input)
	if !strings.HasSuffix(preview, "...") {
		t.Fatalf("expected preview suffix, got %q", preview)
	}
	if !strings.Contains(preview, "測") {
		t.Fatalf("expected preview to retain original runes, got %q", preview)
	}
}
