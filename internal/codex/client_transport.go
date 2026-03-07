package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/config"
	"github.com/gorilla/websocket"
)

func (c *Client) Start(ctx context.Context) error {
	c.stateMu.Lock()
	if c.conn != nil {
		c.stateMu.Unlock()
		return nil
	}
	c.stateMu.Unlock()

	port, err := freePort()
	if err != nil {
		return fmt.Errorf("reserve app-server port: %w", err)
	}
	c.wsURL = fmt.Sprintf("ws://%s:%d", config.DefaultCodexListenHost, port)

	cmd := exec.CommandContext(ctx, c.cfg.Codex.Command, "app-server", "--listen", c.wsURL)
	cmd.Dir = c.paths.Workspace
	cmd.Env = append(os.Environ(), "CODEX_HOME="+c.paths.CodexHome)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("app-server stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("app-server stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start codex app-server: %w", err)
	}
	c.cmd = cmd

	go c.streamLogs("stdout", stdout)
	go c.streamLogs("stderr", stderr)

	conn, err := c.dial(ctx, c.wsURL)
	if err != nil {
		_ = c.cmd.Process.Kill()
		return err
	}

	c.stateMu.Lock()
	c.conn = conn
	c.stateMu.Unlock()

	go c.readLoop()
	if err := c.initialize(ctx); err != nil {
		return err
	}
	return nil
}

func (c *Client) Close() error {
	c.stateMu.Lock()
	conn := c.conn
	cmd := c.cmd
	c.conn = nil
	c.cmd = nil
	c.stateMu.Unlock()

	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	if conn != nil {
		_ = conn.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	return nil
}

func (c *Client) Bootstrap(ctx context.Context) error {
	return c.Start(ctx)
}

func (c *Client) writeJSON(payload any) error {
	c.stateMu.Lock()
	conn := c.conn
	c.stateMu.Unlock()
	if conn == nil {
		return errors.New("app-server connection is not open")
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := conn.WriteJSON(payload); err != nil {
		c.handleConnectionLoss(err)
		return fmt.Errorf("write app-server message: %w", err)
	}
	return nil
}

func (c *Client) handleConnectionLoss(cause error) {
	c.stateMu.Lock()
	conn := c.conn
	cmd := c.cmd
	if conn == nil && cmd == nil && len(c.pending) == 0 && len(c.turns) == 0 {
		c.stateMu.Unlock()
		return
	}
	c.conn = nil
	c.cmd = nil
	pending := c.pending
	turns := c.turns
	c.pending = map[string]chan rpcResponse{}
	c.turns = map[string]*turnWaiter{}
	c.stateMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}

	message := fmt.Sprintf("app-server connection lost: %v", cause)
	for _, ch := range pending {
		if ch == nil {
			continue
		}
		select {
		case ch <- rpcResponse{Error: &rpcError{Message: message}}:
		default:
		}
	}
	for _, waiter := range turns {
		if waiter == nil {
			continue
		}
		select {
		case waiter.completed <- turnResult{Error: errors.New(message)}:
		default:
		}
	}
}

func (c *Client) dial(ctx context.Context, wsURL string) (*websocket.Conn, error) {
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, http.Header{})
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("dial app-server websocket: %w", lastErr)
}

func (c *Client) streamLogs(stream string, reader any) {
	var scanner *bufio.Scanner
	switch r := reader.(type) {
	case interface{ Read([]byte) (int, error) }:
		scanner = bufio.NewScanner(r)
	default:
		return
	}
	for scanner.Scan() {
		line := scanner.Text()
		if stream == "stderr" {
			c.logger.Warn("codex app-server", "stream", stream, "line", line)
			continue
		}
		c.logger.Info("codex app-server", "stream", stream, "line", line)
	}
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
