package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestVoiceSessionTranscriptAndEventLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-time.Minute)
	session := VoiceSession{
		ID:          "voice-1",
		GuildID:     "g-1",
		ChannelID:   "vc-1",
		ChannelName: "voice",
		State:       "listening",
		Source:      "discord_voice",
		StartedAt:   startedAt,
		Metadata:    map[string]any{"connected": true},
	}
	if err := store.UpsertVoiceSession(ctx, session); err != nil {
		t.Fatalf("upsert voice session: %v", err)
	}

	active, ok, err := store.ActiveVoiceSession(ctx, "g-1")
	if err != nil {
		t.Fatalf("active voice session: %v", err)
	}
	if !ok || active.ID != session.ID {
		t.Fatalf("unexpected active session: ok=%v session=%#v", ok, active)
	}

	if err := store.SaveVoiceTranscript(ctx, VoiceTranscriptSegment{
		SessionID:   session.ID,
		SpeakerID:   "owner",
		SpeakerName: "shiyui",
		Role:        "user",
		Content:     "VC で話したい",
		StartedAt:   startedAt,
	}); err != nil {
		t.Fatalf("save voice transcript: %v", err)
	}
	if err := store.SaveVoiceEvent(ctx, VoiceEvent{
		SessionID: session.ID,
		Type:      "join",
		UserID:    "owner",
		ChannelID: "vc-1",
		CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("save voice event: %v", err)
	}

	transcripts, err := store.ListVoiceTranscripts(ctx, session.ID, 10)
	if err != nil {
		t.Fatalf("list voice transcripts: %v", err)
	}
	if len(transcripts) != 1 || transcripts[0].Content != "VC で話したい" {
		t.Fatalf("unexpected transcripts: %#v", transcripts)
	}

	events, err := store.ListVoiceEvents(ctx, session.ID, 10)
	if err != nil {
		t.Fatalf("list voice events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "join" {
		t.Fatalf("unexpected voice events: %#v", events)
	}

	endedAt := startedAt.Add(3 * time.Minute)
	if err := store.EndVoiceSession(ctx, session.ID, endedAt); err != nil {
		t.Fatalf("end voice session: %v", err)
	}
	if _, ok, err := store.ActiveVoiceSession(ctx, "g-1"); err != nil {
		t.Fatalf("active voice session after end: %v", err)
	} else if ok {
		t.Fatalf("expected no active session after end")
	}
}
