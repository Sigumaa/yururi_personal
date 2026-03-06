package bot

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/config"
)

func TestToolDiscoveryTools(t *testing.T) {
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
	}
	app.tools = registry
	app.registerTools(registry)

	ctx := context.Background()

	listResponse, err := registry.Call(ctx, "tools.list", mustJSONRaw(t, map[string]any{}))
	if err != nil {
		t.Fatalf("tools.list: %v", err)
	}
	if !strings.Contains(listResponse.ContentItems[0].Text, "discord__send_message") {
		t.Fatalf("expected discord send tool in list: %#v", listResponse.ContentItems[0])
	}

	searchResponse, err := registry.Call(ctx, "tools.search", mustJSONRaw(t, map[string]any{
		"query": "archive",
	}))
	if err != nil {
		t.Fatalf("tools.search: %v", err)
	}
	if !strings.Contains(searchResponse.ContentItems[0].Text, "discord__archive_channels") {
		t.Fatalf("expected archive tool in search: %#v", searchResponse.ContentItems[0])
	}

	describeResponse, err := registry.Call(ctx, "tools.describe", mustJSONRaw(t, map[string]any{
		"name": "discord__ensure_space",
	}))
	if err != nil {
		t.Fatalf("tools.describe: %v", err)
	}
	if !strings.Contains(describeResponse.ContentItems[0].Text, "internal_name=discord.ensure_space") {
		t.Fatalf("expected internal name in describe output: %#v", describeResponse.ContentItems[0])
	}
}
