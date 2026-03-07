package voice

import (
	"context"
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
	status RealtimeStatus
}

func (r *realtimeStub) Connect(context.Context) error {
	now := time.Now().UTC()
	r.status.Configured = true
	r.status.Connected = true
	r.status.ConnectedAt = &now
	r.status.Model = "gpt-realtime"
	return nil
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

	engine := NewEngine(
		store,
		&discordStub{
			channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
			members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		},
		&realtimeStub{},
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
