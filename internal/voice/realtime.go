package voice

import (
	"context"
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
		if _, _, err := conn.ReadMessage(); err != nil {
			c.mu.Lock()
			if c.conn == conn {
				c.conn = nil
				c.connectedAt = nil
				c.lastError = err.Error()
			}
			c.mu.Unlock()
			return
		}
	}
}
