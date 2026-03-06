package bot

import "testing"

func TestPreviewTextCompactsWhitespace(t *testing.T) {
	got := previewText("  hello\n\nworld   test  ", 100)
	if got != "hello world test" {
		t.Fatalf("unexpected preview: %q", got)
	}
}

func TestPreviewJSONTruncates(t *testing.T) {
	got := previewJSON(map[string]any{
		"message": "abcdefghijklmnopqrstuvwxyz",
	}, 20)
	if got == "" {
		t.Fatal("expected non-empty preview")
	}
	if len(got) > 23 {
		t.Fatalf("expected truncated preview, got %q", got)
	}
}
