package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

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
					c.handleConnectionLoss(err)
					return
				}
				c.logger.Error("app-server read failed", "error", err)
				c.handleConnectionLoss(err)
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
	idValue := decodeIDValue(rawID)
	write := func(result any) {
		c.logger.Debug("rpc server response start", "method", method, "id", previewText(string(rawID), 120), "result_preview", previewJSON(result, 1200))
		if err := c.writeJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      idValue,
			"result":  result,
		}); err != nil {
			c.logger.Error("rpc server response failed", "method", method, "id", previewText(string(rawID), 120), "error", err)
			return
		}
		c.logger.Debug("rpc server response completed", "method", method, "id", previewText(string(rawID), 120))
	}

	switch method {
	case "item/tool/call":
		var request struct {
			ThreadID  string          `json:"threadId"`
			TurnID    string          `json:"turnId"`
			CallID    string          `json:"callId"`
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			write(map[string]any{"success": false, "contentItems": []map[string]any{{"type": "inputText", "text": "invalid tool request"}}})
			return
		}
		internalTool, ok := c.tools.ResolveExternalName(request.Tool)
		if !ok {
			internalTool = request.Tool
		}
		c.logger.Info("codex tool call", "tool", request.Tool, "internal_tool", internalTool, "arguments", strings.TrimSpace(string(request.Arguments)))
		callCtx := WithToolCallMeta(context.Background(), ToolCallMeta{
			ThreadID:  request.ThreadID,
			TurnID:    request.TurnID,
			CallID:    request.CallID,
			Tool:      internalTool,
			StartedAt: time.Now(),
		})
		response, err := c.tools.Call(callCtx, internalTool, request.Arguments)
		if err != nil {
			c.logger.Warn("codex tool call failed", "tool", request.Tool, "internal_tool", internalTool, "error", err)
			write(map[string]any{
				"success": false,
				"contentItems": []map[string]any{
					{"type": "inputText", "text": err.Error()},
				},
			})
			return
		}
		c.logger.Debug("codex tool call completed", "tool", request.Tool, "internal_tool", internalTool, "response_preview", previewToolResponse(response, 1200))
		write(response)
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
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
			answers[question.ID] = map[string]any{"answers": []string{answer}}
		}
		write(map[string]any{"answers": answers})
	case "mcpServer/elicitation/request":
		write(map[string]any{"action": "decline"})
	default:
		_ = c.writeJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      idValue,
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
		if event.Item.Type == "dynamicToolCall" || event.Item.Type == "DynamicToolCall" {
			c.logger.Info("turn item completed", "turn_id", event.TurnID, "item_type", event.Item.Type, "event_preview", previewText(string(params), 1200))
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
	case "item/started":
		var event struct {
			TurnID string `json:"turnId"`
			Item   struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		if err := json.Unmarshal(params, &event); err != nil {
			return
		}
		if event.Item.Type == "dynamicToolCall" || event.Item.Type == "DynamicToolCall" {
			c.logger.Info("turn item started", "turn_id", event.TurnID, "item_type", event.Item.Type, "event_preview", previewText(string(params), 1200))
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
		if event.Turn.Status == "interrupted" {
			c.logger.Info("turn interrupted", "turn_id", event.Turn.ID)
			waiter.completed <- turnResult{Error: ErrTurnInterrupted}
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
