package discord

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestParseVoiceGatewayCloseEventDetects4017(t *testing.T) {
	event, ok := parseVoiceGatewayCloseEvent("voice endpoint example.discord.media websocket closed unexpectantly, websocket: close 4017: E2EE required")
	if !ok {
		t.Fatal("expected voice gateway close event to be detected")
	}
	if event.Code != 4017 {
		t.Fatalf("expected close code 4017, got %d", event.Code)
	}
	if event.Reason != "E2EE required" {
		t.Fatalf("unexpected reason: %q", event.Reason)
	}
	if event.At.IsZero() {
		t.Fatal("expected event timestamp to be populated")
	}
}

func TestClassifyVoiceJoinErrorReturnsDAVEMessageFor4017(t *testing.T) {
	err := classifyVoiceJoinError(
		errors.New("timeout waiting for voice"),
		Channel{ID: "v-1", Name: "voice"},
		voiceGatewayCloseEvent{
			Code:   4017,
			Reason: "E2EE required",
			At:     time.Now().UTC(),
		},
		true,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	for _, want := range []string{"4017", "DAVE/E2EE required", "voice"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in error message, got %s", want, text)
		}
	}
}

func TestClassifyVoiceJoinErrorFallsBackToWrappedError(t *testing.T) {
	err := classifyVoiceJoinError(errors.New("timeout waiting for voice"), Channel{ID: "v-1"}, voiceGatewayCloseEvent{}, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "join voice: timeout waiting for voice") {
		t.Fatalf("unexpected wrapped error: %s", err)
	}
}

type baseVoiceJoinStub struct {
	called bool
}

func (s *baseVoiceJoinStub) ChannelVoiceJoin(guildID string, channelID string, mute bool, deaf bool) (*discordgo.VoiceConnection, error) {
	s.called = true
	return &discordgo.VoiceConnection{GuildID: guildID, ChannelID: channelID}, nil
}

type e2eeVoiceJoinStub struct {
	baseVoiceJoinStub
	e2eeCalled bool
}

func (s *e2eeVoiceJoinStub) ChannelVoiceJoinE2EE(guildID string, channelID string, mute bool, deaf bool) (*discordgo.VoiceConnection, error) {
	s.e2eeCalled = true
	return &discordgo.VoiceConnection{GuildID: guildID, ChannelID: channelID}, nil
}

func TestJoinVoiceConnectionPrefersE2EEWhenAvailable(t *testing.T) {
	session := &e2eeVoiceJoinStub{}
	conn, err := joinVoiceConnection(session, "g-1", "v-1", false, false)
	if err != nil {
		t.Fatalf("joinVoiceConnection: %v", err)
	}
	if conn == nil || !session.e2eeCalled {
		t.Fatalf("expected E2EE join path to be used, conn=%#v called=%t", conn, session.e2eeCalled)
	}
	if session.called {
		t.Fatal("expected standard join path to be skipped when E2EE is available")
	}
}

func TestJoinVoiceConnectionFallsBackToStandardJoin(t *testing.T) {
	session := &baseVoiceJoinStub{}
	conn, err := joinVoiceConnection(session, "g-1", "v-1", false, false)
	if err != nil {
		t.Fatalf("joinVoiceConnection: %v", err)
	}
	if conn == nil || !session.called {
		t.Fatalf("expected standard join path to be used, conn=%#v called=%t", conn, session.called)
	}
}

func TestPrepareVoiceConnectionForLeaveClearsChannelID(t *testing.T) {
	conn := &discordgo.VoiceConnection{GuildID: "g-1", ChannelID: "v-1"}
	prepareVoiceConnectionForLeave(conn)
	if conn.ChannelID != "" {
		t.Fatalf("expected channel id to be cleared before leave, got %q", conn.ChannelID)
	}
}
