package bot

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func TestMediaLoadAttachmentsReturnsImageItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-png"))
	}))
	defer server.Close()

	app := &App{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		http:   server.Client(),
	}
	registry := codex.NewToolRegistry()
	app.registerMediaTools(registry)

	response, err := registry.Call(context.Background(), "media.load_attachments", mustJSONRaw(t, map[string]any{
		"urls": []string{server.URL + "/a.png", server.URL + "/b.jpg"},
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
	if response.ContentItems[1].Type != "inputImage" || response.ContentItems[2].Type != "inputImage" {
		t.Fatalf("expected inputImage items, got %#v", response.ContentItems)
	}
	if response.ContentItems[1].ImageURL == "" || !strings.HasPrefix(response.ContentItems[1].ImageURL, "data:image/png;base64,") {
		t.Fatalf("expected embedded image data url, got %#v", response.ContentItems[1])
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
