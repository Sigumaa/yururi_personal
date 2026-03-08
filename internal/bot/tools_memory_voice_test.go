package bot

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func TestMemoryVoiceTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-tools.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-time.Minute)
	if err := store.UpsertVoiceSession(ctx, memory.VoiceSession{
		ID:          "voice-1",
		GuildID:     "g-1",
		ChannelID:   "vc-1",
		ChannelName: "voice",
		State:       "listening",
		Source:      "discord_voice",
		StartedAt:   startedAt,
	}); err != nil {
		t.Fatalf("upsert voice session: %v", err)
	}
	if err := store.SaveVoiceTranscript(ctx, memory.VoiceTranscriptSegment{
		SessionID:   "voice-1",
		SpeakerID:   "owner",
		SpeakerName: "shiyui",
		Role:        "user",
		Content:     "VC の確認です",
		StartedAt:   startedAt,
	}); err != nil {
		t.Fatalf("save voice transcript: %v", err)
	}

	registry := codex.NewToolRegistry()
	app := &App{
		store:  store,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	app.registerMemoryVoiceTools(registry)

	recent, err := registry.Call(ctx, "memory.recent_voice_transcripts", mustJSONRaw(t, map[string]any{"limit": 5}))
	if err != nil {
		t.Fatalf("recent_voice_transcripts: %v", err)
	}
	if !strings.Contains(recent.ContentItems[0].Text, "VC の確認です") {
		t.Fatalf("unexpected recent voice transcripts output: %s", recent.ContentItems[0].Text)
	}

	search, err := registry.Call(ctx, "memory.search_voice_transcripts", mustJSONRaw(t, map[string]any{"query": "確認"}))
	if err != nil {
		t.Fatalf("search_voice_transcripts: %v", err)
	}
	if !strings.Contains(search.ContentItems[0].Text, "VC の確認です") {
		t.Fatalf("unexpected search voice transcripts output: %s", search.ContentItems[0].Text)
	}
}
