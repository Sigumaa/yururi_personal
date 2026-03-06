package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

type toolTurnProgress struct {
	ModelVisible  bool
	LastUpdatedAt time.Time
}

func (a *App) beforeToolCall(ctx context.Context, toolName string, arguments json.RawMessage, _ codex.ToolResponse, _ error) {
	meta, ok := codex.ToolCallMetaFromContext(ctx)
	if !ok {
		return
	}
	if toolName != "discord.send_message" {
		return
	}
	a.markTurnModelVisible(meta.TurnID)
	a.logger.Debug("tool call marked visible", "tool", toolName, "thread_id", meta.ThreadID, "turn_id", meta.TurnID, "arguments", previewJSON(arguments, 400))
}

func (a *App) afterToolCall(ctx context.Context, toolName string, _ json.RawMessage, response codex.ToolResponse, err error) {
	meta, ok := codex.ToolCallMetaFromContext(ctx)
	if !ok {
		return
	}
	if toolName == "discord.send_message" {
		a.markTurnModelVisible(meta.TurnID)
	}
	if err != nil {
		a.logger.Warn("tool call result", "tool", toolName, "thread_id", meta.ThreadID, "turn_id", meta.TurnID, "error", err)
		return
	}
	a.logger.Debug("tool call result", "tool", toolName, "thread_id", meta.ThreadID, "turn_id", meta.TurnID, "response", previewJSON(response, 800))
}

func requiresVisibleProgress(toolName string) bool {
	switch toolName {
	case "discord.create_category", "discord.create_channel", "discord.rename_channel", "discord.set_channel_topic", "discord.move_channel":
		return true
	default:
		return false
	}
}

func (a *App) requireVisibleProgress(ctx context.Context, toolName string) error {
	if !requiresVisibleProgress(toolName) {
		return nil
	}
	meta, ok := codex.ToolCallMetaFromContext(ctx)
	if !ok {
		return nil
	}
	if a.turnHasModelVisible(meta.TurnID) {
		return nil
	}
	channelID, ok := a.channelIDForThread(meta.ThreadID)
	if !ok || channelID == "" {
		return nil
	}
	return fmt.Errorf("visible progress required before %s: first call %s with a brief update to channel_id=%s, then retry the tool", toolName, codex.ExternalToolName("discord.send_message"), channelID)
}

func (a *App) channelIDForThread(threadID string) (string, bool) {
	a.threadMapMu.Lock()
	defer a.threadMapMu.Unlock()
	channelID, ok := a.threadChannelsByID[threadID]
	return channelID, ok
}

func (a *App) rememberThreadChannel(threadID string, channelID string) {
	a.threadMapMu.Lock()
	defer a.threadMapMu.Unlock()
	if a.threadChannelsByID == nil {
		a.threadChannelsByID = map[string]string{}
	}
	a.threadChannelsByID[threadID] = channelID
}

func (a *App) markTurnModelVisible(turnID string) {
	a.turnProgressMu.Lock()
	defer a.turnProgressMu.Unlock()
	state := a.turnProgress[turnID]
	state.ModelVisible = true
	state.LastUpdatedAt = time.Now()
	a.turnProgress[turnID] = state
	a.pruneTurnProgressLocked()
}

func (a *App) turnHasModelVisible(turnID string) bool {
	a.turnProgressMu.Lock()
	defer a.turnProgressMu.Unlock()
	state := a.turnProgress[turnID]
	state.LastUpdatedAt = time.Now()
	a.turnProgress[turnID] = state
	a.pruneTurnProgressLocked()
	return state.ModelVisible
}

func (a *App) pruneTurnProgressLocked() {
	if len(a.turnProgress) < 128 {
		return
	}
	cutoff := time.Now().Add(-90 * time.Minute)
	for turnID, state := range a.turnProgress {
		if state.LastUpdatedAt.Before(cutoff) {
			delete(a.turnProgress, turnID)
		}
	}
}
