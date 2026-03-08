package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const DefaultRealtimeModel = "gpt-realtime"

type RealtimeClient interface {
	Connect(context.Context) error
	ConfigureSession(context.Context, SessionConfig) error
	AppendInputAudio(context.Context, []byte) error
	CommitInputAudio(context.Context) error
	ClearInputAudio(context.Context) error
	CreateResponse(context.Context) error
	CancelResponse(context.Context) error
	Events() <-chan ServerEvent
	Close() error
	Status() RealtimeStatus
}

type RealtimeOptions struct {
	APIKey string
	Model  string
	URL    string
}

type WebsocketRealtimeClient struct {
	opts   RealtimeOptions
	dialer *websocket.Dialer

	mu          sync.RWMutex
	conn        *websocket.Conn
	connectedAt *time.Time
	lastError   string
	events      chan ServerEvent
}

func NewRealtimeClient(opts RealtimeOptions) *WebsocketRealtimeClient {
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = DefaultRealtimeModel
	}
	if strings.TrimSpace(opts.URL) == "" {
		opts.URL = "wss://api.openai.com/v1/realtime"
	}
	return &WebsocketRealtimeClient{
		opts: opts,
		dialer: &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 15 * time.Second,
		},
		events: make(chan ServerEvent, 64),
	}
}

func (c *WebsocketRealtimeClient) Connect(ctx context.Context) error {
	if !c.Status().Configured {
		return nil
	}
	c.mu.RLock()
	if c.conn != nil {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.opts.APIKey)
	header.Set("OpenAI-Beta", "realtime=v1")
	url := c.opts.URL
	if !strings.Contains(url, "?") {
		url += "?model=" + c.opts.Model
	}
	conn, _, err := c.dialer.DialContext(ctx, url, header)
	if err != nil {
		c.mu.Lock()
		c.lastError = err.Error()
		c.mu.Unlock()
		return fmt.Errorf("connect realtime: %w", err)
	}

	now := time.Now().UTC()
	c.mu.Lock()
	c.conn = conn
	c.connectedAt = &now
	c.lastError = ""
	c.mu.Unlock()

	go c.readLoop()
	return nil
}

func (c *WebsocketRealtimeClient) Close() error {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.connectedAt = nil
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (c *WebsocketRealtimeClient) ConfigureSession(ctx context.Context, session SessionConfig) error {
	return c.send(ctx, map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"type":               "realtime",
			"instructions":       session.Instructions,
			"input_audio_format": session.InputAudioFormat,
			"audio": map[string]any{
				"output": map[string]any{
					"format": map[string]any{
						"type":        session.OutputAudioFormat,
						"sample_rate": session.OutputSampleRate,
					},
					"voice": session.Voice,
				},
			},
			"turn_detection": map[string]any{
				"type":               session.TurnDetection,
				"create_response":    session.CreateResponse,
				"interrupt_response": session.InterruptResponse,
			},
		},
	})
}

func (c *WebsocketRealtimeClient) AppendInputAudio(ctx context.Context, pcm []byte) error {
	return c.send(ctx, map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(pcm),
	})
}

func (c *WebsocketRealtimeClient) CommitInputAudio(ctx context.Context) error {
	return c.send(ctx, map[string]any{"type": "input_audio_buffer.commit"})
}

func (c *WebsocketRealtimeClient) ClearInputAudio(ctx context.Context) error {
	return c.send(ctx, map[string]any{"type": "input_audio_buffer.clear"})
}

func (c *WebsocketRealtimeClient) CreateResponse(ctx context.Context) error {
	return c.send(ctx, map[string]any{"type": "response.create"})
}

func (c *WebsocketRealtimeClient) CancelResponse(ctx context.Context) error {
	return c.send(ctx, map[string]any{"type": "response.cancel"})
}

func (c *WebsocketRealtimeClient) Events() <-chan ServerEvent {
	return c.events
}

func (c *WebsocketRealtimeClient) Status() RealtimeStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return RealtimeStatus{
		Configured:  strings.TrimSpace(c.opts.APIKey) != "",
		Connected:   c.conn != nil,
		Model:       c.opts.Model,
		URL:         c.opts.URL,
		LastError:   c.lastError,
		ConnectedAt: c.connectedAt,
	}
}

func (c *WebsocketRealtimeClient) readLoop() {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return
	}
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			if c.conn == conn {
				c.conn = nil
				c.connectedAt = nil
				c.lastError = err.Error()
			}
			c.mu.Unlock()
			return
		}
		var event ServerEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			continue
		}
		select {
		case c.events <- event:
		default:
		}
	}
}

func (c *WebsocketRealtimeClient) send(ctx context.Context, event any) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal realtime event: %w", err)
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return nil
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
			c.connectedAt = nil
			c.lastError = err.Error()
		}
		c.mu.Unlock()
		return fmt.Errorf("write realtime event: %w", err)
	}
	return nil
}
