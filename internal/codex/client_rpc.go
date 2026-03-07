package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"
)

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
