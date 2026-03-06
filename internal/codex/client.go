package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/config"
	"github.com/gorilla/websocket"
)

type Client struct {
	cfg    config.Config
	paths  config.Paths
	logger *slog.Logger
	tools  *ToolRegistry

	cmd    *exec.Cmd
	conn   *websocket.Conn
	wsURL  string
	closed chan struct{}

	writeMu sync.Mutex
	stateMu sync.Mutex
	nextID  int64
	pending map[string]chan rpcResponse
	turns   map[string]*turnWaiter
}

type BootstrapState struct {
	Config map[string]any
	Skills []SkillEntry
	Apps   []AppEntry
}

type SkillEntry struct {
	CWD    string
	Errors []map[string]any
	Skills []map[string]any
}

type AppEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	IsEnabled    bool   `json:"isEnabled"`
	IsAccessible bool   `json:"isAccessible"`
}

type ThreadSession struct {
	ID string
}

type rpcResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type turnWaiter struct {
	threadID   string
	turnID     string
	deltas     strings.Builder
	texts      []string
	completed  chan turnResult
	receivedAt time.Time
}

type turnResult struct {
	Text  string
	Error error
}

func NewClient(cfg config.Config, paths config.Paths, logger *slog.Logger, tools *ToolRegistry) *Client {
	if tools == nil {
		tools = NewToolRegistry()
	}
	return &Client{
		cfg:     cfg,
		paths:   paths,
		logger:  logger,
		tools:   tools,
		closed:  make(chan struct{}),
		pending: map[string]chan rpcResponse{},
		turns:   map[string]*turnWaiter{},
	}
}

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
	c.wsURL = fmt.Sprintf("ws://%s:%d", c.cfg.Codex.ListenHost, port)

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

func (c *Client) Bootstrap(ctx context.Context) (BootstrapState, error) {
	if err := c.Start(ctx); err != nil {
		return BootstrapState{}, err
	}
	configSnapshot, err := c.ReadConfig(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	skills, err := c.ListSkills(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	apps, err := c.ListApps(ctx)
	if err != nil {
		c.logger.Warn("apps/list failed", "error", err)
	}
	return BootstrapState{
		Config: configSnapshot,
		Skills: skills,
		Apps:   apps,
	}, nil
}

func (c *Client) EnsureThread(ctx context.Context, storedID string, baseInstructions string, developerInstructions string) (ThreadSession, error) {
	if storedID != "" {
		var response struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		err := c.call(ctx, "thread/resume", map[string]any{
			"threadId": storedID,
		}, &response)
		if err == nil && response.Thread.ID != "" {
			return ThreadSession{ID: response.Thread.ID}, nil
		}
		c.logger.Warn("thread/resume failed; starting fresh thread", "thread_id", storedID, "error", err)
	}

	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	params := map[string]any{
		"cwd":                   c.paths.Workspace,
		"approvalPolicy":        c.cfg.Codex.ApprovalPolicy,
		"sandbox":               c.cfg.Codex.SandboxMode,
		"baseInstructions":      baseInstructions,
		"developerInstructions": developerInstructions,
		"serviceName":           c.cfg.AppName,
	}
	if c.cfg.Codex.Model != "" {
		params["model"] = c.cfg.Codex.Model
	}
	if c.cfg.Codex.ModelProvider != "" {
		params["modelProvider"] = c.cfg.Codex.ModelProvider
	}
	if err := c.call(ctx, "thread/start", params, &response); err != nil {
		return ThreadSession{}, err
	}
	return ThreadSession{ID: response.Thread.ID}, nil
}

func (c *Client) ReadConfig(ctx context.Context) (map[string]any, error) {
	var response struct {
		Config map[string]any `json:"config"`
	}
	if err := c.call(ctx, "config/read", map[string]any{
		"cwd": c.paths.Workspace,
	}, &response); err != nil {
		return nil, err
	}
	return response.Config, nil
}

func (c *Client) ListSkills(ctx context.Context) ([]SkillEntry, error) {
	var response struct {
		Data []SkillEntry `json:"data"`
	}
	if err := c.call(ctx, "skills/list", map[string]any{
		"cwds":        []string{c.paths.Workspace},
		"forceReload": true,
	}, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *Client) ListApps(ctx context.Context) ([]AppEntry, error) {
	if !c.cfg.Codex.EnableApps {
		return nil, nil
	}
	var response struct {
		Data []AppEntry `json:"data"`
	}
	if err := c.call(ctx, "app/list", map[string]any{
		"limit":        50,
		"forceRefetch": false,
	}, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *Client) RunTurn(ctx context.Context, threadID string, prompt string) (string, error) {
	return c.runTurn(ctx, threadID, prompt, nil)
}

func (c *Client) RunJSONTurn(ctx context.Context, threadID string, prompt string, outputSchema map[string]any) (string, error) {
	return c.runTurn(ctx, threadID, prompt, outputSchema)
}

func (c *Client) runTurn(ctx context.Context, threadID string, prompt string, outputSchema map[string]any) (string, error) {
	if err := c.Start(ctx); err != nil {
		return "", err
	}
	c.logger.Info("turn start", "thread_id", threadID, "prompt_bytes", len(prompt), "json_schema", outputSchema != nil)
	c.logger.Debug("turn prompt", "thread_id", threadID, "prompt_preview", previewText(prompt, 1800), "schema_preview", previewJSON(outputSchema, 800))
	params := map[string]any{
		"threadId": threadID,
		"input": []map[string]any{
			{
				"type": "text",
				"text": prompt,
			},
		},
		"approvalPolicy": c.cfg.Codex.ApprovalPolicy,
		"sandboxPolicy": map[string]any{
			"type": "dangerFullAccess",
		},
	}
	if outputSchema != nil {
		params["outputSchema"] = outputSchema
	}
	if c.cfg.Codex.Model != "" {
		params["model"] = c.cfg.Codex.Model
	}
	if c.cfg.Codex.ReasoningSummary != "" {
		params["summary"] = c.cfg.Codex.ReasoningSummary
	}
	if c.cfg.Codex.ReasoningEffort != "" {
		params["effort"] = c.cfg.Codex.ReasoningEffort
	}

	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := c.call(ctx, "turn/start", params, &response); err != nil {
		return "", err
	}
	c.logger.Debug("turn started", "thread_id", threadID, "turn_id", response.Turn.ID)

	waiter := &turnWaiter{
		threadID:   threadID,
		turnID:     response.Turn.ID,
		completed:  make(chan turnResult, 1),
		receivedAt: time.Now(),
	}
	c.stateMu.Lock()
	c.turns[response.Turn.ID] = waiter
	c.stateMu.Unlock()
	defer func() {
		c.stateMu.Lock()
		delete(c.turns, response.Turn.ID)
		c.stateMu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-waiter.completed:
		if result.Error != nil {
			return "", result.Error
		}
		c.logger.Info("turn completed", "thread_id", threadID, "turn_id", response.Turn.ID, "response_bytes", len(result.Text), "elapsed", time.Since(waiter.receivedAt))
		c.logger.Debug("turn output", "thread_id", threadID, "turn_id", response.Turn.ID, "response_preview", previewText(result.Text, 1800))
		if outputSchema != nil {
			return normalizeJSONText(result.Text), nil
		}
		return strings.TrimSpace(result.Text), nil
	}
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	id := strconv.FormatInt(atomic.AddInt64(&c.nextID, 1), 10)
	ch := make(chan rpcResponse, 1)
	c.stateMu.Lock()
	c.pending[id] = ch
	c.stateMu.Unlock()
	defer func() {
		c.stateMu.Lock()
		delete(c.pending, id)
		c.stateMu.Unlock()
	}()

	c.logger.Debug("rpc call start", "method", method, "id", id, "params_preview", previewJSON(params, 1200))
	if err := c.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-ch:
		if response.Error != nil {
			c.logger.Warn("rpc call failed", "method", method, "id", id, "error", response.Error.Message, "error_data", previewText(string(response.Error.Data), 1200))
			return fmt.Errorf("%s: %s", method, response.Error.Message)
		}
		if out == nil || len(response.Result) == 0 {
			c.logger.Debug("rpc call completed", "method", method, "id", id, "result_preview", "")
			return nil
		}
		c.logger.Debug("rpc call completed", "method", method, "id", id, "result_preview", previewText(string(response.Result), 1200))
		if err := json.Unmarshal(response.Result, out); err != nil {
			return fmt.Errorf("decode %s response: %w", method, err)
		}
		return nil
	}
}

func (c *Client) initialize(ctx context.Context) error {
	var initResp map[string]any
	if err := c.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    c.cfg.AppName,
			"title":   "yururi",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}, &initResp); err != nil {
		return fmt.Errorf("initialize app-server: %w", err)
	}
	return c.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
	})
}

func (c *Client) readLoop() {
	for {
		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := c.conn.ReadJSON(&envelope); err != nil {
			select {
			case <-c.closed:
				return
			default:
				if errors.Is(err, net.ErrClosed) ||
					websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) ||
					strings.Contains(err.Error(), "unexpected EOF") {
					c.logger.Info("app-server connection closed", "error", err)
					return
				}
				c.logger.Error("app-server read failed", "error", err)
				return
			}
		}

		switch {
		case envelope.Method != "" && len(envelope.ID) > 0:
			go c.handleServerRequest(envelope.ID, envelope.Method, envelope.Params)
		case envelope.Method != "":
			c.handleNotification(envelope.Method, envelope.Params)
		default:
			id := decodeID(envelope.ID)
			c.stateMu.Lock()
			ch := c.pending[id]
			c.stateMu.Unlock()
			if ch != nil {
				ch <- rpcResponse{ID: envelope.ID, Result: envelope.Result, Error: envelope.Error}
			}
		}
	}
}

func (c *Client) handleServerRequest(rawID json.RawMessage, method string, params json.RawMessage) {
	id := decodeID(rawID)
	write := func(result any) {
		_ = c.writeJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  result,
		})
	}

	switch method {
	case "item/tool/call":
		var request struct {
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			write(map[string]any{"success": false, "contentItems": []map[string]any{{"type": "inputText", "text": "invalid tool request"}}})
			return
		}
		c.logger.Info("codex tool call", "tool", request.Tool, "arguments", strings.TrimSpace(string(request.Arguments)))
		response, err := c.tools.Call(context.Background(), request.Tool, request.Arguments)
		if err != nil {
			c.logger.Warn("codex tool call failed", "tool", request.Tool, "error", err)
			write(map[string]any{
				"success": false,
				"contentItems": []map[string]any{
					{"type": "inputText", "text": err.Error()},
				},
			})
			return
		}
		c.logger.Debug("codex tool call completed", "tool", request.Tool, "response_preview", previewToolResponse(response, 1200))
		write(response)
	case "item/commandExecution/requestApproval":
		write(map[string]any{"decision": "acceptForSession"})
	case "item/fileChange/requestApproval":
		write(map[string]any{"decision": "acceptForSession"})
	case "item/tool/requestUserInput":
		var request struct {
			Questions []struct {
				ID      string `json:"id"`
				Options []struct {
					Label string `json:"label"`
				} `json:"options"`
			} `json:"questions"`
		}
		_ = json.Unmarshal(params, &request)
		answers := map[string]any{}
		for _, question := range request.Questions {
			answer := ""
			if len(question.Options) > 0 {
				answer = question.Options[0].Label
			}
			c.logger.Info("auto answered requestUserInput", "question_id", question.ID, "answer", answer)
			answers[question.ID] = map[string]any{
				"answers": []string{answer},
			}
		}
		write(map[string]any{"answers": answers})
	case "mcpServer/elicitation/request":
		write(map[string]any{"action": "decline"})
	default:
		_ = c.writeJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]any{
				"code":    -32601,
				"message": "unsupported server request",
			},
		})
	}
}

func (c *Client) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "item/agentMessage/delta", "agent_message_delta":
		var event struct {
			Delta  string `json:"delta"`
			TurnID string `json:"turnId"`
		}
		if err := json.Unmarshal(params, &event); err != nil {
			return
		}
		c.stateMu.Lock()
		waiter := c.turns[event.TurnID]
		c.stateMu.Unlock()
		if waiter != nil {
			waiter.deltas.WriteString(event.Delta)
			c.logger.Debug("turn delta", "turn_id", event.TurnID, "delta_bytes", len(event.Delta), "delta_preview", previewText(event.Delta, 240))
		}
	case "item/completed":
		var event struct {
			TurnID string `json:"turnId"`
			Item   struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(params, &event); err != nil {
			return
		}
		if event.Item.Type != "agentMessage" && event.Item.Type != "AgentMessage" {
			return
		}
		c.stateMu.Lock()
		waiter := c.turns[event.TurnID]
		c.stateMu.Unlock()
		if waiter != nil && strings.TrimSpace(event.Item.Text) != "" {
			waiter.texts = append(waiter.texts, strings.TrimSpace(event.Item.Text))
			c.logger.Debug("turn item completed", "turn_id", event.TurnID, "item_type", event.Item.Type, "text_preview", previewText(event.Item.Text, 800))
		}
	case "turn/completed":
		var event struct {
			Turn struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		}
		if err := json.Unmarshal(params, &event); err != nil {
			return
		}
		c.stateMu.Lock()
		waiter := c.turns[event.Turn.ID]
		c.stateMu.Unlock()
		if waiter == nil {
			return
		}
		if event.Turn.Error != nil {
			c.logger.Warn("turn completed with error", "turn_id", event.Turn.ID, "status", event.Turn.Status, "error", event.Turn.Error.Message)
			waiter.completed <- turnResult{Error: errors.New(event.Turn.Error.Message)}
			return
		}
		c.logger.Debug("turn completion notification", "turn_id", event.Turn.ID, "status", event.Turn.Status)
		waiter.completed <- turnResult{Text: resolveTurnText(waiter)}
	case "error":
		var event map[string]any
		_ = json.Unmarshal(params, &event)
		c.logger.Warn("app-server notification error", "event", event)
	}
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
		return fmt.Errorf("write app-server message: %w", err)
	}
	return nil
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

func decodeID(raw json.RawMessage) string {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var asInt int64
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return strconv.FormatInt(asInt, 10)
	}
	return strings.TrimSpace(string(raw))
}

func normalizeJSONText(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

func resolveTurnText(waiter *turnWaiter) string {
	if waiter == nil {
		return ""
	}
	if len(waiter.texts) > 0 {
		return strings.Join(waiter.texts, "\n")
	}
	return waiter.deltas.String()
}
