package bot

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func TestRequireVisibleProgressBlocksMutatingToolUntilModelSpeaks(t *testing.T) {
	app := &App{
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		threadChannelsByID: map[string]string{"thread-1": "channel-1"},
		turnProgress:       map[string]toolTurnProgress{},
	}
	ctx := codex.WithToolCallMeta(context.Background(), codex.ToolCallMeta{
		ThreadID:  "thread-1",
		TurnID:    "turn-1",
		Tool:      "discord.create_channel",
		StartedAt: time.Now(),
	})

	err := app.requireVisibleProgress(ctx, "discord.create_channel")
	if err == nil {
		t.Fatal("expected progress requirement error")
	}
	if !strings.Contains(err.Error(), "discord__send_message") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "channel_id=channel-1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireVisibleProgressAllowsMutatingToolAfterModelMessage(t *testing.T) {
	app := &App{
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		threadChannelsByID: map[string]string{"thread-1": "channel-1"},
		turnProgress:       map[string]toolTurnProgress{},
	}
	ctx := codex.WithToolCallMeta(context.Background(), codex.ToolCallMeta{
		ThreadID:  "thread-1",
		TurnID:    "turn-1",
		Tool:      "discord.send_message",
		StartedAt: time.Now(),
	})
	app.beforeToolCall(ctx, "discord.send_message", json.RawMessage(`{"channel_id":"channel-1","content":"やってみますね"}`), codex.ToolResponse{}, nil)

	mutatingCtx := codex.WithToolCallMeta(context.Background(), codex.ToolCallMeta{
		ThreadID:  "thread-1",
		TurnID:    "turn-1",
		Tool:      "discord.create_channel",
		StartedAt: time.Now(),
	})
	if err := app.requireVisibleProgress(mutatingCtx, "discord.create_channel"); err != nil {
		t.Fatalf("requireVisibleProgress: %v", err)
	}
}
