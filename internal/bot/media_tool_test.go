package bot

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func TestMediaLoadAttachmentsReturnsImageItems(t *testing.T) {
	app := &App{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	registry := codex.NewToolRegistry()
	app.registerMediaTools(registry)

	response, err := registry.Call(context.Background(), "media.load_attachments", mustJSONRaw(t, map[string]any{
		"urls": []string{"https://example.com/a.png", "https://example.com/b.jpg"},
	}))
	if err != nil {
		t.Fatalf("call media.load_attachments: %v", err)
	}
	if !response.Success {
		t.Fatal("expected success")
	}
	if len(response.ContentItems) != 3 {
		t.Fatalf("expected 3 content items, got %d", len(response.ContentItems))
	}
	if response.ContentItems[1].Type != "imageUrl" || response.ContentItems[2].Type != "imageUrl" {
		t.Fatalf("expected imageUrl items, got %#v", response.ContentItems)
	}
}

func mustJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
