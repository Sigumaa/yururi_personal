package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) registerCoreJobTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "jobs.list",
		Description: "登録済み job の一覧を確認する",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "job kind で絞る"),
			fieldSchema("state", "string", "pending, running, failed, completed で絞る"),
			fieldSchema("channel_id", "string", "チャンネル ID で絞る"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind      string `json:"kind"`
			State     string `json:"state"`
			ChannelID string `json:"channel_id"`
			Limit     int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 32
		}
		list, err := a.store.ListJobs(ctx, input.Kind, jobs.State(strings.TrimSpace(input.State)), input.ChannelID, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(list))
		for _, job := range list {
			lines = append(lines, fmt.Sprintf("- %s %s state=%s channel=%s next=%s", job.Kind, job.ID, job.State, job.ChannelID, job.NextRunAt.Format(time.RFC3339)))
		}
		if len(lines) == 0 {
			lines = append(lines, "no jobs")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule",
		Description: "job を登録または更新する",
		InputSchema: objectSchema(
			fieldSchema("id", "string", "job ID"),
			fieldSchema("kind", "string", "job の種別"),
			fieldSchema("title", "string", "job の表示名"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
			fieldSchema("payload", "object", "job payload"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID        string         `json:"id"`
			Kind      string         `json:"kind"`
			Title     string         `json:"title"`
			ChannelID string         `json:"channel_id"`
			Schedule  string         `json:"schedule"`
			Payload   map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Title) == "" {
			return codex.ToolResponse{}, errors.New("kind and title are required")
		}
		if input.ID == "" {
			input.ID = jobID(input.Kind)
		}
		if input.Schedule == "" {
			input.Schedule = defaultWatchSchedule
		}
		job := jobs.NewJob(input.ID, input.Kind, input.Title, input.ChannelID, input.Schedule, input.Payload)
		if input.Kind == "codex_release_watch" && job.Payload["repo"] == nil {
			job.Payload["repo"] = "openai/codex"
		}
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.cancel",
		Description: "job を停止する",
		InputSchema: objectSchema(fieldSchema("id", "string", "停止する job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if input.ID == "" {
			return codex.ToolResponse{}, errors.New("id is required")
		}
		if err := a.store.UpdateJobState(ctx, input.ID, jobs.StateCompleted, time.Now().UTC(), "cancelled", nil); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("cancelled"), nil
	})
}
