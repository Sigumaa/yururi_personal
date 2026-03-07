package codex

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/config"
)

func (c *Client) RunTurn(ctx context.Context, threadID string, prompt string) (string, error) {
	return c.runTurn(ctx, threadID, []InputItem{TextInput(prompt)}, nil, TurnOptions{})
}

func (c *Client) RunInputTurn(ctx context.Context, threadID string, input []InputItem) (string, error) {
	return c.runTurn(ctx, threadID, input, nil, TurnOptions{})
}

func (c *Client) RunInputTurnWithOptions(ctx context.Context, threadID string, input []InputItem, opts TurnOptions) (string, error) {
	return c.runTurn(ctx, threadID, input, nil, opts)
}

func (c *Client) RunJSONTurn(ctx context.Context, threadID string, prompt string, outputSchema map[string]any) (string, error) {
	return c.runTurn(ctx, threadID, []InputItem{TextInput(prompt)}, outputSchema, TurnOptions{})
}

func (c *Client) InterruptTurn(ctx context.Context, threadID string, turnID string) error {
	if strings.TrimSpace(threadID) == "" || strings.TrimSpace(turnID) == "" {
		return errors.New("thread_id and turn_id are required")
	}
	c.logger.Info("turn interrupt requested", "thread_id", threadID, "turn_id", turnID)
	if err := c.call(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	}, nil); err != nil {
		return err
	}
	c.logger.Info("turn interrupt sent", "thread_id", threadID, "turn_id", turnID)
	return nil
}

func (c *Client) InterruptActiveTurn(ctx context.Context, threadID string) (string, bool, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	var active *turnWaiter
	for _, waiter := range c.turns {
		if waiter == nil || waiter.threadID != threadID {
			continue
		}
		if active == nil || waiter.receivedAt.After(active.receivedAt) {
			active = waiter
		}
	}
	if active == nil {
		return "", false, nil
	}
	if err := c.interruptTurnLocked(ctx, threadID, active.turnID); err != nil {
		return active.turnID, true, err
	}
	return active.turnID, true, nil
}

func (c *Client) runTurn(ctx context.Context, threadID string, input []InputItem, outputSchema map[string]any, opts TurnOptions) (string, error) {
	if err := c.Start(ctx); err != nil {
		return "", err
	}
	c.logger.Info("turn start", "thread_id", threadID, "input_count", len(input), "json_schema", outputSchema != nil)
	c.logger.Debug("turn input", "thread_id", threadID, "input_preview", previewJSON(input, 1800), "schema_preview", previewJSON(outputSchema, 800))

	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := c.call(ctx, "turn/start", c.turnParams(threadID, input, outputSchema, opts), &response); err != nil {
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
		interruptCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := c.InterruptTurn(interruptCtx, threadID, response.Turn.ID)
		cancel()
		if err != nil {
			c.logger.Warn("turn interrupt after context done failed", "thread_id", threadID, "turn_id", response.Turn.ID, "error", err)
		}
		select {
		case result := <-waiter.completed:
			if err := normalizeInterruptedResult(result); err != nil {
				return "", err
			}
		case <-time.After(3 * time.Second):
		}
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

func (c *Client) turnParams(threadID string, input []InputItem, outputSchema map[string]any, opts TurnOptions) map[string]any {
	params := map[string]any{
		"threadId":       threadID,
		"input":          input,
		"approvalPolicy": config.DefaultCodexApprovalPolicy,
		"sandboxPolicy": map[string]any{
			"type": "dangerFullAccess",
		},
		"summary": config.DefaultCodexReasoningSummary,
	}
	if outputSchema != nil {
		params["outputSchema"] = outputSchema
	}

	effort := strings.TrimSpace(opts.Effort)
	if effort == "" {
		effort = config.DefaultCodexReasoningEffort
	}
	if effort != "" {
		params["effort"] = effort
	}
	return params
}

func (c *Client) interruptTurnLocked(ctx context.Context, threadID string, turnID string) error {
	c.stateMu.Unlock()
	err := c.InterruptTurn(ctx, threadID, turnID)
	c.stateMu.Lock()
	return err
}

func normalizeInterruptedResult(result turnResult) error {
	if errors.Is(result.Error, ErrTurnInterrupted) {
		return ErrTurnInterrupted
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
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
