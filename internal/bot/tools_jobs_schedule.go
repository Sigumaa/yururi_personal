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

func (a *App) registerJobScheduleTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_release_watch",
		Description: "GitHub リリース監視 job を作る",
		InputSchema: objectSchema(
			fieldSchema("repo", "string", "owner/repo"),
			fieldSchema("channel_id", "string", "通知先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Repo      string `json:"repo"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Repo == "" {
			input.Repo = "openai/codex"
		}
		if input.Schedule == "" {
			input.Schedule = defaultWatchSchedule
		}
		job := jobs.NewJob(jobID("release-watch"), "codex_release_watch", "release watch", input.ChannelID, input.Schedule, map[string]any{
			"repo": input.Repo,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_summary",
		Description: "summary / review 系の定期 job を作る",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "daily_summary, weekly_review, monthly_review, growth_log, open_loop_review"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "任意の Go duration"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind      string `json:"kind"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if !isSupportedSummaryKind(input.Kind) {
			return codex.ToolResponse{}, errors.New("unsupported summary kind")
		}
		if input.Schedule == "" {
			input.Schedule = defaultSummarySchedule(input.Kind)
		}
		job := jobs.NewJob(jobID(strings.ReplaceAll(input.Kind, "_", "-")), input.Kind, input.Kind, input.ChannelID, input.Schedule, nil)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_url_watch",
		Description: "任意 URL の更新監視 job を作る",
		InputSchema: objectSchema(
			fieldSchema("url", "string", "監視対象 URL"),
			fieldSchema("channel_id", "string", "通知先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			URL       string `json:"url"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.URL) == "" {
			return codex.ToolResponse{}, errors.New("url is required")
		}
		if input.Schedule == "" {
			input.Schedule = defaultWatchSchedule
		}
		job := jobs.NewJob(jobID("url-watch"), "url_watch", "url watch", input.ChannelID, input.Schedule, map[string]any{
			"url": input.URL,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_codex_task",
		Description: "バックグラウンドで Codex task を 1 回実行する job を作る",
		InputSchema: objectSchema(
			fieldSchema("title", "string", "job の表示名"),
			fieldSchema("prompt", "string", "バックグラウンドで実行する依頼文"),
			fieldSchema("channel_id", "string", "結果を返すチャンネル ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Title     string `json:"title"`
			Prompt    string `json:"prompt"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Prompt) == "" {
			return codex.ToolResponse{}, errors.New("title and prompt are required")
		}
		job := jobs.NewJob(jobID("codex-task"), "codex_background_task", input.Title, input.ChannelID, "10s", map[string]any{
			"prompt": input.Prompt,
			"goal":   input.Title,
		})
		job.NextRunAt = time.Now().UTC().Add(10 * time.Second)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_periodic_codex_task",
		Description: "Codex task を定期実行する generic job を作る",
		InputSchema: objectSchema(
			fieldSchema("title", "string", "job の表示名"),
			fieldSchema("prompt", "string", "定期的に実行する依頼文"),
			fieldSchema("channel_id", "string", "結果を返すチャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Title     string `json:"title"`
			Prompt    string `json:"prompt"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Prompt) == "" {
			return codex.ToolResponse{}, errors.New("title and prompt are required")
		}
		if strings.TrimSpace(input.Schedule) == "" {
			input.Schedule = "6h"
		}
		job := jobs.NewJob(jobID("codex-periodic"), "codex_periodic_task", input.Title, input.ChannelID, input.Schedule, map[string]any{
			"prompt": input.Prompt,
			"goal":   input.Title,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_reminder",
		Description: "あとで一度だけ返す reminder / follow-up を作る",
		InputSchema: objectSchema(
			fieldSchema("title", "string", "reminder の表示名"),
			fieldSchema("message", "string", "投稿する本文"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("after", "string", "今からどれくらい後か。Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Title     string `json:"title"`
			Message   string `json:"message"`
			ChannelID string `json:"channel_id"`
			After     string `json:"after"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Message) == "" || strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("message and channel_id are required")
		}
		if strings.TrimSpace(input.Title) == "" {
			input.Title = "reminder"
		}
		if strings.TrimSpace(input.After) == "" {
			input.After = "30m"
		}
		delay := mustDuration(input.After, 30*time.Minute)
		job := jobs.NewJob(jobID("reminder"), "reminder", input.Title, input.ChannelID, input.After, map[string]any{
			"content": input.Message,
		})
		job.NextRunAt = time.Now().UTC().Add(delay)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_space_review",
		Description: "空間整理候補を定期的に見直す job を作る",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
			fieldSchema("since_hours", "integer", "何時間ぶんの活動を見るか"),
			fieldSchema("title", "string", "任意の表示名"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ChannelID  string `json:"channel_id"`
			Schedule   string `json:"schedule"`
			SinceHours int    `json:"since_hours"`
			Title      string `json:"title"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("channel_id is required")
		}
		if strings.TrimSpace(input.Schedule) == "" {
			input.Schedule = "24h"
		}
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		if strings.TrimSpace(input.Title) == "" {
			input.Title = "space review"
		}
		job := jobs.NewJob(jobID("space-review"), "space_review", input.Title, input.ChannelID, input.Schedule, map[string]any{
			"since_hours": input.SinceHours,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})
}

func isSupportedSummaryKind(kind string) bool {
	switch kind {
	case "daily_summary", "weekly_review", "monthly_review", "growth_log", "open_loop_review":
		return true
	default:
		return false
	}
}

func defaultSummarySchedule(kind string) string {
	switch kind {
	case "daily_summary", "growth_log":
		return "24h"
	case "weekly_review":
		return "168h"
	case "monthly_review":
		return "720h"
	case "open_loop_review":
		return "48h"
	default:
		return ""
	}
}
