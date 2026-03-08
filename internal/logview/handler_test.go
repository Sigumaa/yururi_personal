package logview

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestHandlerFormatsInlineAndBlockAttrs(t *testing.T) {
	var out strings.Builder
	handler := NewHandler(&out, Options{
		Level:      slog.LevelDebug,
		Color:      false,
		TimeFormat: "15:04:05",
	})
	logger := slog.New(handler)
	logger.Info("codex tool call",
		"tool", "discord__send_message",
		"thread_id", "thread-1",
		"arguments", `{"channel_id":"1","content":"hello world"}`,
	)

	got := out.String()
	for _, want := range []string{
		"INF codex tool call",
		"tool",
		"discord__send_message",
		"thread_id",
		"thread-1",
		"arguments",
		`"channel_id": "1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in log output, got %s", want, got)
		}
	}
}

func TestHandlerFlattensGroups(t *testing.T) {
	var out strings.Builder
	handler := NewHandler(&out, Options{
		Level:      slog.LevelDebug,
		Color:      false,
		TimeFormat: "15:04:05",
	})

	record := slog.NewRecord(time.Date(2026, 3, 8, 14, 12, 7, 0, time.UTC), slog.LevelInfo, "discordgo voice", 0)
	record.AddAttrs(
		slog.Group("voice",
			slog.Int("close_code", 4017),
			slog.String("close_reason", "E2EE/DAVE protocol required"),
		),
	)

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"voice.close_code",
		"4017",
		"voice.close_reason",
		"E2EE/DAVE protocol required",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in grouped output, got %s", want, got)
		}
	}
}

func TestHandlerPrettyPrintsJSONStrings(t *testing.T) {
	var out strings.Builder
	handler := NewHandler(&out, Options{
		Level:      slog.LevelDebug,
		Color:      false,
		TimeFormat: "15:04:05",
	})
	logger := slog.New(handler)
	logger.Debug("payload", "arguments", `{"tool":"discord.send","nested":{"ok":true}}`)

	got := out.String()
	for _, want := range []string{
		"arguments",
		`"tool": "discord.send"`,
		`"nested": {`,
		`"ok": true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in pretty JSON output, got %s", want, got)
		}
	}
}
