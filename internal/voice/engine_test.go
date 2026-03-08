package voice

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

type discordStub struct {
	channel discordsvc.Channel
	members []discordsvc.VoiceMember
	joined  bool
	left    bool
}

func (d *discordStub) GetChannel(context.Context, string) (discordsvc.Channel, error) {
	return d.channel, nil
}

func (d *discordStub) JoinVoice(context.Context, string, string, bool, bool) (discordsvc.VoiceSession, error) {
	d.joined = true
	return discordsvc.VoiceSession{
		GuildID:     "g-1",
		ChannelID:   d.channel.ID,
		ChannelName: d.channel.Name,
		Connected:   true,
	}, nil
}

func (d *discordStub) LeaveVoice(context.Context, string) error {
	d.left = true
	return nil
}

func (d *discordStub) CurrentVoiceSession(context.Context, string) (discordsvc.VoiceSession, bool, error) {
	return discordsvc.VoiceSession{
		GuildID:     "g-1",
		ChannelID:   d.channel.ID,
		ChannelName: d.channel.Name,
		Connected:   d.joined && !d.left,
	}, d.joined && !d.left, nil
}

func (d *discordStub) VoiceChannelMembers(context.Context, string, string) ([]discordsvc.VoiceMember, error) {
	return d.members, nil
}

type realtimeStub struct {
	status    RealtimeStatus
	config    SessionConfig
	events    chan ServerEvent
	appended  [][]byte
	committed int
	cleared   int
	created   int
	canceled  int
}

func (r *realtimeStub) Connect(context.Context) error {
	now := time.Now().UTC()
	r.status.Configured = true
	r.status.Connected = true
	r.status.ConnectedAt = &now
	r.status.Model = "gpt-realtime"
	if r.events == nil {
		r.events = make(chan ServerEvent, 16)
	}
	return nil
}

func (r *realtimeStub) ConfigureSession(_ context.Context, config SessionConfig) error {
	r.config = config
	return nil
}

func (r *realtimeStub) AppendInputAudio(_ context.Context, pcm []byte) error {
	r.appended = append(r.appended, append([]byte(nil), pcm...))
	return nil
}

func (r *realtimeStub) CommitInputAudio(context.Context) error {
	r.committed++
	return nil
}

func (r *realtimeStub) ClearInputAudio(context.Context) error {
	r.cleared++
	return nil
}

func (r *realtimeStub) CreateResponse(context.Context) error {
	r.created++
	return nil
}

func (r *realtimeStub) CancelResponse(context.Context) error {
	r.canceled++
	return nil
}

func (r *realtimeStub) Events() <-chan ServerEvent {
	if r.events == nil {
		r.events = make(chan ServerEvent, 16)
	}
	return r.events
}

func (r *realtimeStub) Close() error {
	r.status.Connected = false
	r.status.ConnectedAt = nil
	return nil
}

func (r *realtimeStub) Status() RealtimeStatus {
	return r.status
}

func TestEngineJoinStatusAndLeave(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	realtime := &realtimeStub{}
	engine := NewEngine(
		store,
		&discordStub{
			channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
			members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		},
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	session, err := engine.Join(context.Background(), "g-1", "vc-1")
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if session.ChannelID != "vc-1" || !session.Realtime.Connected {
		t.Fatalf("unexpected session: %#v", session)
	}
	if realtime.config.Voice == "" || realtime.config.Instructions == "" {
		t.Fatalf("expected realtime session to be configured: %#v", realtime.config)
	}

	current, ok, err := engine.Status(context.Background(), "g-1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !ok || current.ChannelID != "vc-1" {
		t.Fatalf("unexpected current session: ok=%v session=%#v", ok, current)
	}

	if err := engine.HandleVoiceStateUpdate(context.Background(), &discordgo.VoiceStateUpdate{
		VoiceState: &discordgo.VoiceState{GuildID: "g-1", UserID: "owner", ChannelID: "vc-1"},
	}); err != nil {
		t.Fatalf("handle voice state update: %v", err)
	}

	if err := engine.Leave(context.Background(), "g-1", "done"); err != nil {
		t.Fatalf("leave: %v", err)
	}

	if _, ok, err := store.ActiveVoiceSession(context.Background(), "g-1"); err != nil {
		t.Fatalf("active voice session after leave: %v", err)
	} else if ok {
		t.Fatalf("expected no active voice session after leave")
	}
}

func TestEngineRecordsRealtimeTranscriptsAndMirrorsMessages(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-events.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		&discordStub{
			channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
			members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		},
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	session, err := engine.Join(context.Background(), "g-1", "vc-1")
	if err != nil {
		t.Fatalf("join: %v", err)
	}

	realtime.events <- mustServerEvent(t, map[string]any{
		"type":                 "conversation.item.input_audio_transcription.completed",
		"conversation_item_id": "item-user-1",
		"transcript":           "こんばんは",
	})
	realtime.events <- mustServerEvent(t, map[string]any{
		"type":        "response.audio_transcript.done",
		"response_id": "resp-1",
		"item_id":     "item-assistant-1",
		"transcript":  "はい、聞こえていますよ。",
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		transcripts, err := store.ListVoiceTranscripts(context.Background(), session.ID, 10)
		if err != nil {
			t.Fatalf("list voice transcripts: %v", err)
		}
		if len(transcripts) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for transcripts, got %d", len(transcripts))
		}
		time.Sleep(20 * time.Millisecond)
	}

	messages, err := store.RecentMessages(context.Background(), session.ChannelID, 10)
	if err != nil {
		t.Fatalf("recent messages: %v", err)
	}
	if len(messages) == 0 {
		t.Fatalf("expected mirrored voice transcripts to appear in raw messages")
	}
}

func mustServerEvent(t *testing.T, payload map[string]any) ServerEvent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal server event: %v", err)
	}
	var event ServerEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("unmarshal server event: %v", err)
	}
	return event
}
