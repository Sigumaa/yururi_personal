package voice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRealtimeClientConnectsToConfiguredServer(t *testing.T) {
	upgrader := websocket.Upgrader{}
	received := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Bearer test-key") {
			t.Fatalf("missing auth header: %s", r.Header.Get("Authorization"))
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var event map[string]any
			if err := json.Unmarshal(payload, &event); err != nil {
				t.Fatalf("unmarshal client event: %v", err)
			}
			select {
			case received <- event:
			default:
			}
			if event["type"] == "session.update" {
				if err := conn.WriteJSON(map[string]any{"type": "session.updated"}); err != nil {
					t.Fatalf("write session.updated: %v", err)
				}
			}
		}
	}))
	defer server.Close()

	client := NewRealtimeClient(RealtimeOptions{
		APIKey: "test-key",
		Model:  "gpt-realtime",
		URL:    "ws" + strings.TrimPrefix(server.URL, "http"),
	})

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect realtime: %v", err)
	}
	if err := client.ConfigureSession(context.Background(), DefaultSessionConfig("voice")); err != nil {
		t.Fatalf("configure realtime session: %v", err)
	}
	select {
	case event := <-received:
		session, ok := event["session"].(map[string]any)
		if !ok {
			t.Fatalf("expected session payload, got %#v", event["session"])
		}
		if got := event["type"]; got != "session.update" {
			t.Fatalf("expected session.update event, got %#v", got)
		}
		if got := session["output_modalities"]; got == nil {
			t.Fatalf("expected output_modalities in session payload")
		}
		audio, ok := session["audio"].(map[string]any)
		if !ok {
			t.Fatalf("expected audio config, got %#v", session["audio"])
		}
		input, ok := audio["input"].(map[string]any)
		if !ok {
			t.Fatalf("expected audio.input, got %#v", audio["input"])
		}
		output, ok := audio["output"].(map[string]any)
		if !ok {
			t.Fatalf("expected audio.output, got %#v", audio["output"])
		}
		inputFormat, ok := input["format"].(map[string]any)
		if !ok || inputFormat["type"] != "audio/pcm" {
			t.Fatalf("expected audio.input.format.type=audio/pcm, got %#v", input["format"])
		}
		if output["format"] != "pcm16" {
			t.Fatalf("expected audio.output.format=pcm16, got %#v", output["format"])
		}
		if output["voice"] != defaultVoiceName {
			t.Fatalf("expected audio.output.voice=%s, got %#v", defaultVoiceName, output["voice"])
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected to capture session.update payload")
	}
	select {
	case event := <-client.Events():
		if event.Type != "session.updated" {
			t.Fatalf("unexpected event type: %s", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected realtime event after session update")
	}
	status := client.Status()
	if !status.Configured || !status.Connected {
		t.Fatalf("unexpected status after connect: %#v", status)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close realtime: %v", err)
	}
}

func TestRealtimeClientAppliesPendingSessionBeforeAudioAppend(t *testing.T) {
	upgrader := websocket.Upgrader{}
	received := make(chan map[string]any, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var event map[string]any
			if err := json.Unmarshal(payload, &event); err != nil {
				t.Fatalf("unmarshal client event: %v", err)
			}
			received <- event
		}
	}))
	defer server.Close()

	client := NewRealtimeClient(RealtimeOptions{
		APIKey: "test-key",
		Model:  "gpt-realtime",
		URL:    "ws" + strings.TrimPrefix(server.URL, "http"),
	})
	cfg := DefaultSessionConfig("voice")
	client.sessionConfig = &cfg
	client.sessionDirty = true

	if err := client.AppendInputAudio(context.Background(), []byte{0x01, 0x02}); err != nil {
		t.Fatalf("append input audio: %v", err)
	}

	first := <-received
	second := <-received
	if first["type"] != "session.update" {
		t.Fatalf("expected first event to be session.update, got %#v", first["type"])
	}
	if second["type"] != "input_audio_buffer.append" {
		t.Fatalf("expected second event to be input_audio_buffer.append, got %#v", second["type"])
	}
}
