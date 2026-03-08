package discord

import (
	"errors"
	"strings"
	"testing"
	"time"
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
