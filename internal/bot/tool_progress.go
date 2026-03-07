package bot

import (
	"context"
	"encoding/json"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func (a *App) beforeToolCall(ctx context.Context, toolName string, arguments json.RawMessage, _ codex.ToolResponse, _ error) {
	meta, ok := codex.ToolCallMetaFromContext(ctx)
	if !ok {
		return
	}
	if toolName != "discord.send_message" {
		return
	}
	a.logger.Debug("tool call marked visible", "tool", toolName, "thread_id", meta.ThreadID, "turn_id", meta.TurnID, "arguments", previewJSON(arguments, 400))
}

func (a *App) afterToolCall(ctx context.Context, toolName string, _ json.RawMessage, response codex.ToolResponse, err error) {
	meta, ok := codex.ToolCallMetaFromContext(ctx)
	if !ok {
		return
	}
	if err != nil {
		a.logger.Warn("tool call result", "tool", toolName, "thread_id", meta.ThreadID, "turn_id", meta.TurnID, "error", err)
		return
	}
	a.logger.Debug("tool call result", "tool", toolName, "thread_id", meta.ThreadID, "turn_id", meta.TurnID, "response", previewJSON(response, 800))
}
