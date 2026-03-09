package bot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/config"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	presencemodel "github.com/Sigumaa/yururi_personal/internal/presence"
	"github.com/Sigumaa/yururi_personal/internal/voice"
	"github.com/bwmarrin/discordgo"
)

type discordToolsStub struct {
	channels          map[string]discordsvc.Channel
	nextID            int
	ensuredCategories []string
	createdChannels   []discordsvc.ChannelSpec
	movedChannels     []string
	renamedChannels   []string
}

func newDiscordToolsStub() *discordToolsStub {
	return &discordToolsStub{
		channels: map[string]discordsvc.Channel{
			"c-1": {ID: "c-1", Name: "general", Type: discordgo.ChannelTypeGuildText},
			"c-2": {ID: "c-2", Name: "notes", Type: discordgo.ChannelTypeGuildText},
			"v-1": {ID: "v-1", Name: "voice", Type: discordgo.ChannelTypeGuildVoice},
		},
		nextID: 10,
	}
}

func (d *discordToolsStub) Open() error                                                            { return nil }
func (d *discordToolsStub) Close() error                                                           { return nil }
func (d *discordToolsStub) AddMessageHandler(func(*discordgo.Session, *discordgo.MessageCreate))   {}
func (d *discordToolsStub) AddPresenceHandler(func(*discordgo.Session, *discordgo.PresenceUpdate)) {}
func (d *discordToolsStub) AddVoiceStateHandler(func(*discordgo.Session, *discordgo.VoiceStateUpdate)) {
}
func (d *discordToolsStub) SendMessage(context.Context, string, string) (string, error) {
	return "m-1", nil
}
func (d *discordToolsStub) CreateTextChannel(context.Context, string, discordsvc.ChannelSpec) (discordsvc.Channel, error) {
	return discordsvc.Channel{}, nil
}
func (d *discordToolsStub) EnsureTextChannel(_ context.Context, _ string, spec discordsvc.ChannelSpec) (discordsvc.Channel, error) {
	d.nextID++
	channel := discordsvc.Channel{
		ID:       fmt.Sprintf("ch-%d", d.nextID),
		Name:     spec.Name,
		Topic:    spec.Topic,
		ParentID: spec.ParentID,
		Type:     discordgo.ChannelTypeGuildText,
	}
	d.channels[channel.ID] = channel
	d.createdChannels = append(d.createdChannels, spec)
	return channel, nil
}
func (d *discordToolsStub) EnsureCategory(_ context.Context, _ string, name string) (discordsvc.Channel, error) {
	for _, channel := range d.channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory && channel.Name == name {
			return channel, nil
		}
	}
	d.nextID++
	channel := discordsvc.Channel{ID: fmt.Sprintf("cat-%d", d.nextID), Name: name, Type: discordgo.ChannelTypeGuildCategory}
	d.channels[channel.ID] = channel
	d.ensuredCategories = append(d.ensuredCategories, name)
	return channel, nil
}
func (d *discordToolsStub) MoveChannel(_ context.Context, channelID string, parentID string) error {
	channel, ok := d.channels[channelID]
	if !ok {
		return errors.New("unknown channel")
	}
	channel.ParentID = parentID
	d.channels[channelID] = channel
	d.movedChannels = append(d.movedChannels, channelID)
	return nil
}
func (d *discordToolsStub) GetChannel(_ context.Context, channelID string) (discordsvc.Channel, error) {
	channel, ok := d.channels[channelID]
	if !ok {
		return discordsvc.Channel{}, errors.New("unknown channel")
	}
	return channel, nil
}
func (d *discordToolsStub) RenameChannel(_ context.Context, channelID string, name string) (discordsvc.Channel, error) {
	channel, ok := d.channels[channelID]
	if !ok {
		return discordsvc.Channel{}, errors.New("unknown channel")
	}
	channel.Name = name
	d.channels[channelID] = channel
	d.renamedChannels = append(d.renamedChannels, channelID)
	return channel, nil
}
func (d *discordToolsStub) SetChannelTopic(_ context.Context, channelID string, topic string) (discordsvc.Channel, error) {
	channel, ok := d.channels[channelID]
	if !ok {
		return discordsvc.Channel{}, errors.New("unknown channel")
	}
	channel.Topic = topic
	d.channels[channelID] = channel
	return channel, nil
}
func (d *discordToolsStub) RecentMessages(context.Context, string, int) ([]discordsvc.Message, error) {
	return nil, nil
}
func (d *discordToolsStub) ListChannels(context.Context, string) ([]discordsvc.Channel, error) {
	out := make([]discordsvc.Channel, 0, len(d.channels))
	for _, channel := range d.channels {
		out = append(out, channel)
	}
	return out, nil
}
func (d *discordToolsStub) ListVoiceChannels(context.Context, string) ([]discordsvc.VoiceChannel, error) {
	return []discordsvc.VoiceChannel{
		{ID: "v-1", Name: "voice", MemberCount: 1, Members: []discordsvc.VoiceMember{{UserID: "owner", Username: "owner", ChannelID: "v-1"}}},
	}, nil
}
func (d *discordToolsStub) VoiceChannelMembers(context.Context, string, string) ([]discordsvc.VoiceMember, error) {
	return []discordsvc.VoiceMember{{UserID: "owner", Username: "owner", ChannelID: "v-1"}}, nil
}
func (d *discordToolsStub) CurrentMemberVoiceState(context.Context, string, string) (discordsvc.VoiceState, bool, error) {
	return discordsvc.VoiceState{UserID: "owner", Username: "owner", ChannelID: "v-1"}, true, nil
}
func (d *discordToolsStub) JoinVoice(context.Context, string, string, bool, bool) (discordsvc.VoiceSession, error) {
	return discordsvc.VoiceSession{GuildID: "g-1", ChannelID: "v-1", ChannelName: "voice", Connected: true}, nil
}
func (d *discordToolsStub) LeaveVoice(context.Context, string) error { return nil }
func (d *discordToolsStub) VoiceAudioPackets(context.Context, string) (<-chan discordsvc.VoicePacket, error) {
	return make(chan discordsvc.VoicePacket), nil
}
func (d *discordToolsStub) SendVoiceOpus(context.Context, string, []byte) error  { return nil }
func (d *discordToolsStub) SetVoiceSpeaking(context.Context, string, bool) error { return nil }
func (d *discordToolsStub) CurrentVoiceSession(context.Context, string) (discordsvc.VoiceSession, bool, error) {
	return discordsvc.VoiceSession{GuildID: "g-1", ChannelID: "v-1", ChannelName: "voice", Connected: true}, true, nil
}
func (d *discordToolsStub) CurrentPresence(context.Context, string, string) (discordsvc.Presence, error) {
	start := time.Date(2026, 3, 8, 1, 2, 3, 0, time.UTC)
	end := start.Add(3 * time.Minute)
	return discordsvc.Presence{
		UserID: "owner",
		Status: "online",
		Activities: []presencemodel.Activity{
			{
				Name:      "Spotify",
				Type:      "listening",
				Details:   "Blue Train",
				State:     "John Coltrane",
				LargeText: "Blue Train",
				StartAt:   &start,
				EndAt:     &end,
			},
		},
	}, nil
}
func (d *discordToolsStub) SelfChannelPermissions(context.Context, string) (discordsvc.PermissionSnapshot, error) {
	return discordsvc.PermissionSnapshot{}, nil
}
func (d *discordToolsStub) SelfUserID() string { return "bot" }

func TestDiscordEnsureSpaceArchiveAndIdleTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Add(-10 * 24 * time.Hour)
	if err := store.SaveMessage(ctx, memory.Message{
		ID:          "old-1",
		ChannelID:   "c-1",
		ChannelName: "general",
		AuthorID:    "owner",
		AuthorName:  "shiyui",
		Content:     "old",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if err := store.UpsertChannelProfile(ctx, memory.ChannelProfile{
		ChannelID:           "c-2",
		Name:                "notes",
		Kind:                "monologue",
		ReplyAggressiveness: 0.2,
		AutonomyLevel:       0.9,
		SummaryCadence:      "daily",
	}); err != nil {
		t.Fatalf("profile: %v", err)
	}

	discord := newDiscordToolsStub()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg: config.Config{
			Discord: config.DiscordConfig{GuildID: "g-1"},
		},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:     time.UTC,
		store:   store,
		discord: discord,
	}
	app.registerDiscordExtraTools(registry)

	if _, err := registry.Call(ctx, "discord.ensure_space", mustJSONRaw(t, map[string]any{
		"category_name": "test-lab",
		"channels": []map[string]any{
			{"name": "links", "topic": "watch links"},
			{"name": "logs", "topic": "run logs"},
		},
	})); err != nil {
		t.Fatalf("ensure_space: %v", err)
	}
	if len(discord.ensuredCategories) == 0 || len(discord.createdChannels) != 2 {
		t.Fatalf("unexpected ensure_space behavior: categories=%v channels=%v", discord.ensuredCategories, discord.createdChannels)
	}

	if _, err := registry.Call(ctx, "discord.archive_channels", mustJSONRaw(t, map[string]any{
		"channel_ids":           []string{"c-2"},
		"archive_category_name": "archive",
		"rename_prefix":         "archived",
	})); err != nil {
		t.Fatalf("archive_channels: %v", err)
	}
	archived, err := discord.GetChannel(ctx, "c-2")
	if err != nil {
		t.Fatalf("get archived channel: %v", err)
	}
	if !strings.HasPrefix(archived.Name, "archived-") {
		t.Fatalf("expected renamed archived channel, got %#v", archived)
	}
	if archived.ParentID == "" {
		t.Fatalf("expected archived channel to be moved, got %#v", archived)
	}

	response, err := registry.Call(ctx, "discord.describe_idle_channels", mustJSONRaw(t, map[string]any{
		"since_hours": 24,
	}))
	if err != nil {
		t.Fatalf("describe_idle_channels: %v", err)
	}
	if !strings.Contains(response.ContentItems[0].Text, "notes") {
		t.Fatalf("expected idle notes channel, got %#v", response.ContentItems[0])
	}
}

func TestDiscordGetMemberPresenceIncludesRichDetails(t *testing.T) {
	discord := newDiscordToolsStub()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:     config.Config{Discord: config.DiscordConfig{GuildID: "g-1"}},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		discord: discord,
	}
	app.registerCoreDiscordTools(registry)

	response, err := registry.Call(context.Background(), "discord.get_member_presence", mustJSONRaw(t, map[string]any{
		"user_id": "owner",
	}))
	if err != nil {
		t.Fatalf("get_member_presence: %v", err)
	}
	text := response.ContentItems[0].Text
	for _, want := range []string{"status=online", "type=listening", "name=Spotify", "details=Blue Train", "state=John Coltrane", "activity_summaries=listening / Spotify (Blue Train - John Coltrane)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in presence output, got %s", want, text)
		}
	}
}

func TestDiscordVoiceTools(t *testing.T) {
	discord := newDiscordToolsStub()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{GuildID: "g-1", OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		store: func() *memory.Store {
			store, err := memory.Open(filepath.Join(t.TempDir(), "voice.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			return store
		}(),
		discord: discord,
	}
	defer app.store.Close()
	app.voice = voice.NewEngine(app.store, discord, &voiceRealtimeStub{}, "owner", slog.New(slog.NewTextHandler(io.Discard, nil)))
	app.registerCoreDiscordTools(registry)

	listResponse, err := registry.Call(context.Background(), "discord.list_voice_channels", mustJSONRaw(t, map[string]any{}))
	if err != nil {
		t.Fatalf("list_voice_channels: %v", err)
	}
	if !strings.Contains(listResponse.ContentItems[0].Text, "voice") {
		t.Fatalf("unexpected voice channel list: %s", listResponse.ContentItems[0].Text)
	}

	if _, err := registry.Call(context.Background(), "discord.join_voice", mustJSONRaw(t, map[string]any{
		"user_id": "owner",
	})); err != nil {
		t.Fatalf("join_voice: %v", err)
	}

	statusResponse, err := registry.Call(context.Background(), "discord.voice_session_status", mustJSONRaw(t, map[string]any{}))
	if err != nil {
		t.Fatalf("voice_session_status: %v", err)
	}
	if !strings.Contains(statusResponse.ContentItems[0].Text, "channel=voice (v-1)") {
		t.Fatalf("unexpected voice session status: %s", statusResponse.ContentItems[0].Text)
	}

	if _, err := registry.Call(context.Background(), "discord.interrupt_voice", mustJSONRaw(t, map[string]any{
		"reason": "test",
	})); err != nil {
		t.Fatalf("interrupt_voice: %v", err)
	}

	if _, err := registry.Call(context.Background(), "discord.leave_voice", mustJSONRaw(t, map[string]any{})); err != nil {
		t.Fatalf("leave_voice: %v", err)
	}
}

type voiceRealtimeStub struct{}

func (voiceRealtimeStub) Connect(context.Context) error                               { return nil }
func (voiceRealtimeStub) ConfigureSession(context.Context, voice.SessionConfig) error { return nil }
func (voiceRealtimeStub) AppendInputAudio(context.Context, []byte) error              { return nil }
func (voiceRealtimeStub) CommitInputAudio(context.Context) error                      { return nil }
func (voiceRealtimeStub) ClearInputAudio(context.Context) error                       { return nil }
func (voiceRealtimeStub) CreateResponse(context.Context) error                        { return nil }
func (voiceRealtimeStub) CancelResponse(context.Context) error                        { return nil }
func (voiceRealtimeStub) TruncateConversationItem(context.Context, string, int, int) error {
	return nil
}
func (voiceRealtimeStub) Events() <-chan voice.ServerEvent { return make(chan voice.ServerEvent) }
func (voiceRealtimeStub) Close() error                     { return nil }
func (voiceRealtimeStub) Status() voice.RealtimeStatus {
	return voice.RealtimeStatus{Configured: true, Connected: true, Model: voice.DefaultRealtimeModel}
}
