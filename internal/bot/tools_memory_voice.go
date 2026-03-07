package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func (a *App) registerMemoryVoiceTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.list_voice_sessions",
		Description: "保存済みの VC session 一覧を新しい順に見る",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "返す件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		sessions, err := a.store.RecentVoiceSessions(ctx, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(sessions))
		for _, session := range sessions {
			lines = append(lines, fmt.Sprintf("- %s channel=%s (%s) state=%s started=%s ended=%s", session.ID, session.ChannelName, session.ChannelID, session.State, session.StartedAt.Format(time.RFC3339), optionalTimeString(session.EndedAt)))
		}
		if len(lines) == 0 {
			lines = append(lines, "no voice sessions")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_voice_events",
		Description: "保存済みの VC event 一覧を見る",
		InputSchema: objectSchema(
			fieldSchema("session_id", "string", "対象 session ID"),
			fieldSchema("limit", "integer", "返す件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			SessionID string `json:"session_id"`
			Limit     int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.SessionID) == "" {
			return codex.ToolResponse{}, fmt.Errorf("session_id is required")
		}
		events, err := a.store.ListVoiceEvents(ctx, input.SessionID, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(events))
		for _, event := range events {
			lines = append(lines, fmt.Sprintf("- %s at=%s user=%s channel=%s", event.Type, event.CreatedAt.Format(time.RFC3339), event.UserID, event.ChannelID))
		}
		if len(lines) == 0 {
			lines = append(lines, "no voice events")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_voice_transcripts",
		Description: "保存済みの VC transcript を時系列で見る",
		InputSchema: objectSchema(
			fieldSchema("session_id", "string", "対象 session ID"),
			fieldSchema("limit", "integer", "返す件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			SessionID string `json:"session_id"`
			Limit     int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.SessionID) == "" {
			return codex.ToolResponse{}, fmt.Errorf("session_id is required")
		}
		segments, err := a.store.ListVoiceTranscripts(ctx, input.SessionID, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(segments))
		for _, segment := range segments {
			lines = append(lines, fmt.Sprintf("- %s/%s: %s", segment.Role, segment.SpeakerName, segment.Content))
		}
		if len(lines) == 0 {
			lines = append(lines, "no voice transcripts")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
}

func optionalTimeString(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.Format(time.RFC3339)
}
