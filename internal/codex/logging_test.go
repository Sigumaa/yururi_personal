package codex

import "testing"

func TestPreviewTextCompactsWhitespace(t *testing.T) {
	got := previewText(" hello\nworld  test ", 100)
	if got != "hello world test" {
		t.Fatalf("unexpected preview: %q", got)
	}
}

func TestPreviewToolResponseIsNotEmpty(t *testing.T) {
	got := previewToolResponse(ToolResponse{
		Success: true,
		ContentItems: []ToolContentItem{
			{Type: "inputText", Text: "done"},
		},
	}, 80)
	if got == "" {
		t.Fatal("expected preview")
	}
}
