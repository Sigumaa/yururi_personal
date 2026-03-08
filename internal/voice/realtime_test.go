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
