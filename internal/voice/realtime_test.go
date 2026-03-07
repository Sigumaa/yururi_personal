package voice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			if _, _, err := conn.ReadMessage(); err != nil {
				return
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
	status := client.Status()
	if !status.Configured || !status.Connected {
		t.Fatalf("unexpected status after connect: %#v", status)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close realtime: %v", err)
	}
}
