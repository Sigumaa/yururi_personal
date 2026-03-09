package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

type discordStub struct {
	channel    discordsvc.Channel
	members    []discordsvc.VoiceMember
	voiceState map[string]discordsvc.VoiceState
	selfUserID string
	joinCalls  int
	joined     bool
	left       bool
	packets    chan discordsvc.VoicePacket
	sent       [][]byte
	speaking   []bool
}

func (d *discordStub) GetChannel(context.Context, string) (discordsvc.Channel, error) {
	return d.channel, nil
}

func (d *discordStub) JoinVoice(context.Context, string, string, bool, bool) (discordsvc.VoiceSession, error) {
	d.joinCalls++
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

func (d *discordStub) CurrentMemberVoiceState(_ context.Context, _ string, userID string) (discordsvc.VoiceState, bool, error) {
	if d.voiceState == nil {
		return discordsvc.VoiceState{}, false, nil
	}
	state, ok := d.voiceState[userID]
	return state, ok, nil
}

func (d *discordStub) VoiceChannelMembers(context.Context, string, string) ([]discordsvc.VoiceMember, error) {
	return d.members, nil
}

func (d *discordStub) VoiceAudioPackets(context.Context, string) (<-chan discordsvc.VoicePacket, error) {
	if d.packets == nil {
		d.packets = make(chan discordsvc.VoicePacket, 16)
	}
	return d.packets, nil
}

func (d *discordStub) SendVoiceOpus(_ context.Context, _ string, opus []byte) error {
	d.sent = append(d.sent, append([]byte(nil), opus...))
	return nil
}

func (d *discordStub) SetVoiceSpeaking(_ context.Context, _ string, speaking bool) error {
	d.speaking = append(d.speaking, speaking)
	return nil
}

func (d *discordStub) SelfUserID() string {
	return d.selfUserID
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
	truncated []truncateCall
}

type truncateCall struct {
	itemID       string
	contentIndex int
	audioEndMS   int
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

func (r *realtimeStub) TruncateConversationItem(_ context.Context, itemID string, contentIndex int, audioEndMS int) error {
	r.truncated = append(r.truncated, truncateCall{
		itemID:       itemID,
		contentIndex: contentIndex,
		audioEndMS:   audioEndMS,
	})
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
	if realtime.config.Voice != defaultVoiceName {
		t.Fatalf("expected default voice %s, got %#v", defaultVoiceName, realtime.config.Voice)
	}
	if realtime.config.CreateResponse || realtime.config.InterruptResponse {
		t.Fatalf("expected manual response control, got %#v", realtime.config)
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

func TestEngineJoinLogsVoiceStateSnapshot(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	engine := NewEngine(
		store,
		&discordStub{
			channel:    discordsvc.Channel{ID: "vc-1", Name: "voice"},
			members:    []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
			selfUserID: "bot",
			voiceState: map[string]discordsvc.VoiceState{
				"bot":   {UserID: "bot", Username: "yururi", ChannelID: "vc-1"},
				"owner": {UserID: "owner", Username: "shiyui", ChannelID: "vc-1", SelfMuted: true},
			},
		},
		&realtimeStub{},
		"owner",
		logger,
	)

	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	output := logs.String()
	if !strings.Contains(output, "voice join state snapshot") {
		t.Fatalf("expected join state snapshot log, got %q", output)
	}
	if !strings.Contains(output, "bot_state_known=true") || !strings.Contains(output, "owner_state_known=true") {
		t.Fatalf("expected bot and owner voice state in logs, got %q", output)
	}
}

func TestEngineJoinReusesActiveSessionForSameChannel(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-reuse.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel:    discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members:    []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		selfUserID: "bot",
		voiceState: map[string]discordsvc.VoiceState{
			"bot":   {UserID: "bot", Username: "yururi", ChannelID: "vc-1"},
			"owner": {UserID: "owner", Username: "shiyui", ChannelID: "vc-1"},
		},
	}
	engine := NewEngine(
		store,
		discord,
		&realtimeStub{},
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	first, err := engine.Join(context.Background(), "g-1", "vc-1")
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	second, err := engine.Join(context.Background(), "g-1", "vc-1")
	if err != nil {
		t.Fatalf("second join: %v", err)
	}

	if discord.joinCalls != 1 {
		t.Fatalf("expected one discord join call, got %d", discord.joinCalls)
	}
	if second.ID != first.ID {
		t.Fatalf("expected reused session id %q, got %q", first.ID, second.ID)
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
		"type":        "response.output_audio_transcript.done",
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

func TestRealtimeAudioDeltaIsEncodedForDiscord(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-audio.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	pcm := make([]int16, 480)
	for i := range pcm {
		pcm[i] = 1000
	}
	realtime.events <- mustServerEvent(t, map[string]any{
		"type":  "response.output_audio.delta",
		"delta": base64.StdEncoding.EncodeToString(samplesToPCM16Bytes(pcm)),
	})
	realtime.events <- mustServerEvent(t, map[string]any{
		"type":        "response.done",
		"response_id": "resp-1",
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(discord.sent) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for encoded voice output")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(discord.speaking) == 0 || !discord.speaking[0] {
		t.Fatalf("expected voice speaking to start before sending audio, calls=%v", discord.speaking)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		if len(discord.speaking) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for voice speaking to stop, calls=%v", discord.speaking)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if discord.speaking[len(discord.speaking)-1] {
		t.Fatalf("expected final voice speaking state to be false, calls=%v", discord.speaking)
	}
}

func TestOwnerVoicePacketIsForwardedToRealtime(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-input.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		packets: make(chan discordsvc.VoicePacket, 16),
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	codec, err := newAudioRuntime()
	if err != nil {
		t.Fatalf("new audio runtime: %v", err)
	}
	defer codec.Close()
	frame := make([]int16, discordFrameSamples*discordChannels)
	for i := range frame {
		frame[i] = 500
	}
	encoded := make([]byte, maxOpusPacketSize)
	n, err := codec.encoder.Encode(frame, encoded)
	if err != nil {
		t.Fatalf("encode test opus: %v", err)
	}
	discord.packets <- discordsvc.VoicePacket{
		GuildID:   "g-1",
		ChannelID: "vc-1",
		UserID:    "owner",
		Username:  "shiyui",
		Opus:      append([]byte(nil), encoded[:n]...),
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(realtime.appended) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for realtime audio append")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSpeechStoppedRequestsResponseWithoutCommit(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-turn.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		packets: make(chan discordsvc.VoicePacket, 16),
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	realtime.events <- mustServerEvent(t, map[string]any{
		"type": "input_audio_buffer.speech_stopped",
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		if realtime.created > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for response request, committed=%d created=%d", realtime.committed, realtime.created)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if realtime.committed != 0 {
		t.Fatalf("expected no manual commit with VAD enabled, committed=%d", realtime.committed)
	}
	session, ok, err := engine.Status(context.Background(), "g-1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !ok || session.State != SessionStateThinking {
		t.Fatalf("expected session state to move to thinking after response request, got ok=%v state=%q", ok, session.State)
	}
}

func TestOwnerVoicePacketWithoutSpeakerMappingIsInferredWhenAlone(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-infer-owner.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{
			{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"},
			{UserID: "bot", Username: "yururi", ChannelID: "vc-1", Bot: true},
		},
		packets: make(chan discordsvc.VoicePacket, 16),
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	codec, err := newAudioRuntime()
	if err != nil {
		t.Fatalf("new audio runtime: %v", err)
	}
	defer codec.Close()
	frame := make([]int16, discordFrameSamples*discordChannels)
	for i := range frame {
		frame[i] = 500
	}
	encoded := make([]byte, maxOpusPacketSize)
	n, err := codec.encoder.Encode(frame, encoded)
	if err != nil {
		t.Fatalf("encode test opus: %v", err)
	}
	discord.packets <- discordsvc.VoicePacket{
		GuildID:   "g-1",
		ChannelID: "vc-1",
		SSRC:      42,
		Opus:      append([]byte(nil), encoded[:n]...),
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(realtime.appended) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for inferred owner packet forwarding")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestUnknownSpeakerPacketIsIgnoredWhenMultipleHumansPresent(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-ignore-unknown.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{
			{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"},
			{UserID: "friend", Username: "friend", ChannelID: "vc-1"},
			{UserID: "bot", Username: "yururi", ChannelID: "vc-1", Bot: true},
		},
		packets: make(chan discordsvc.VoicePacket, 16),
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	codec, err := newAudioRuntime()
	if err != nil {
		t.Fatalf("new audio runtime: %v", err)
	}
	defer codec.Close()
	frame := make([]int16, discordFrameSamples*discordChannels)
	for i := range frame {
		frame[i] = 500
	}
	encoded := make([]byte, maxOpusPacketSize)
	n, err := codec.encoder.Encode(frame, encoded)
	if err != nil {
		t.Fatalf("encode test opus: %v", err)
	}
	discord.packets <- discordsvc.VoicePacket{
		GuildID:   "g-1",
		ChannelID: "vc-1",
		SSRC:      42,
		Opus:      append([]byte(nil), encoded[:n]...),
	}

	time.Sleep(300 * time.Millisecond)
	if len(realtime.appended) != 0 {
		t.Fatalf("expected unknown speaker packet to be ignored when multiple humans are present")
	}
}

func TestSpeechStoppedSkipsWhenResponseAlreadyStarted(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-turn-response.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		packets: make(chan discordsvc.VoicePacket, 16),
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	realtime.events <- mustServerEvent(t, map[string]any{
		"type":        "response.created",
		"response_id": "resp-1",
	})
	realtime.events <- mustServerEvent(t, map[string]any{
		"type": "input_audio_buffer.speech_stopped",
	})

	time.Sleep(200 * time.Millisecond)
	if realtime.committed != 0 || realtime.created != 0 {
		t.Fatalf("expected speech_stopped to skip request after response.created, committed=%d created=%d", realtime.committed, realtime.created)
	}
}

func TestSpeechStartedDoesNotResetSpeakingState(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-speech-started.db"))
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
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := engine.setSessionState(context.Background(), "g-1", SessionStateSpeaking); err != nil {
		t.Fatalf("set speaking state: %v", err)
	}

	realtime.events <- mustServerEvent(t, map[string]any{
		"type": "input_audio_buffer.speech_started",
	})

	time.Sleep(100 * time.Millisecond)

	session, ok, err := engine.Status(context.Background(), "g-1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !ok || session.State != SessionStateSpeaking {
		t.Fatalf("expected speech_started not to reset speaking state, got ok=%v state=%q", ok, session.State)
	}
}

func TestComfortNoisePacketDoesNotInterruptOrAppend(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-silence.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		packets: make(chan discordsvc.VoicePacket, 16),
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	runtime, ok := engine.sessionRuntime("g-1")
	if !ok {
		t.Fatalf("expected active runtime")
	}
	runtime.session.State = SessionStateSpeaking

	discord.packets <- discordsvc.VoicePacket{
		GuildID:   "g-1",
		ChannelID: "vc-1",
		UserID:    "owner",
		Username:  "shiyui",
		Opus:      []byte{0xF8, 0xFF, 0xFE},
	}

	time.Sleep(100 * time.Millisecond)

	if realtime.canceled != 0 {
		t.Fatalf("expected comfort noise not to interrupt, canceled=%d", realtime.canceled)
	}
	if len(realtime.appended) != 0 {
		t.Fatalf("expected comfort noise not to append audio, appended=%d", len(realtime.appended))
	}
}

func TestHandleRealtimeSessionUpdatedLogsEffectiveConfig(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-session-updated.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		&discordStub{
			channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
			members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
		},
		realtime,
		"owner",
		logger,
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	realtime.events <- mustServerEvent(t, map[string]any{
		"type": "session.updated",
		"session": map[string]any{
			"instructions": "あなたの名前はゆるりです。",
			"audio": map[string]any{
				"input": map[string]any{
					"turn_detection": map[string]any{
						"type":               "semantic_vad",
						"eagerness":          "low",
						"create_response":    false,
						"interrupt_response": false,
					},
					"transcription": map[string]any{
						"model": "gpt-4o-mini-transcribe",
					},
				},
				"output": map[string]any{
					"voice": "marin",
				},
			},
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		out := logs.String()
		if strings.Contains(out, "voice realtime session updated") {
			if !strings.Contains(out, "voice=marin") || !strings.Contains(out, "turn_detection=semantic_vad") {
				t.Fatalf("expected effective realtime config in logs, got %q", out)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for realtime config log, got %q", logs.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSpeechStartedDoesNotCancelResponseDirectly(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-speech-started.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}

	if err := engine.handleRealtimeEvent(context.Background(), "g-1", "session-1", mustServerEvent(t, map[string]any{
		"type": "input_audio_buffer.speech_started",
	})); err != nil {
		t.Fatalf("handle speech_started: %v", err)
	}
	if realtime.canceled != 0 {
		t.Fatalf("expected speech_started not to cancel directly, canceled=%d", realtime.canceled)
	}
}

func TestInterruptTruncatesCurrentAssistantAudio(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "voice-truncate.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	discord := &discordStub{
		channel: discordsvc.Channel{ID: "vc-1", Name: "voice"},
		members: []discordsvc.VoiceMember{{UserID: "owner", Username: "shiyui", ChannelID: "vc-1"}},
	}
	realtime := &realtimeStub{events: make(chan ServerEvent, 16)}
	engine := NewEngine(
		store,
		discord,
		realtime,
		"owner",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if _, err := engine.Join(context.Background(), "g-1", "vc-1"); err != nil {
		t.Fatalf("join: %v", err)
	}
	runtime, ok := engine.sessionRuntime("g-1")
	if !ok {
		t.Fatalf("expected runtime")
	}

	if err := engine.handleRealtimeEvent(context.Background(), "g-1", runtime.session.ID, mustServerEvent(t, map[string]any{
		"type": "response.output_item.added",
		"item": map[string]any{
			"id":   "item-assistant-1",
			"role": "assistant",
		},
	})); err != nil {
		t.Fatalf("handle output item added: %v", err)
	}
	runtime.outputAudioMS = 480
	runtime.playbackActive = true

	if err := engine.Interrupt(context.Background(), "g-1", "owner_voice_activity"); err != nil {
		t.Fatalf("interrupt: %v", err)
	}

	if realtime.canceled != 1 {
		t.Fatalf("expected cancel on interrupt, got %d", realtime.canceled)
	}
	if len(realtime.truncated) != 1 {
		t.Fatalf("expected one truncate call, got %d", len(realtime.truncated))
	}
	if realtime.truncated[0].itemID != "item-assistant-1" || realtime.truncated[0].audioEndMS != 480 {
		t.Fatalf("unexpected truncate payload: %#v", realtime.truncated[0])
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
