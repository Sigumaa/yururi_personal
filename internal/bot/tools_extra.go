package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func (a *App) registerExtraTools(registry *codex.ToolRegistry) {
	a.registerMemoryExtraTools(registry)
	a.registerJobExtraTools(registry)
	a.registerDiscordExtraTools(registry)
	a.registerWebTools(registry)
	a.registerMediaTools(registry)
}

func (a *App) registerMemoryExtraTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.list_facts",
		Description: "kind 単位または全体で fact を一覧する",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "fact の種別。省略可"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind  string `json:"kind"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		facts, err := a.store.ListFacts(ctx, input.Kind, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(facts) == 0 {
			return textTool("no facts"), nil
		}
		lines := make([]string, 0, len(facts))
		for _, fact := range facts {
			lines = append(lines, fmt.Sprintf("- %s/%s: %s", fact.Kind, fact.Key, fact.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.delete_fact",
		Description: "不要になった fact を削除する",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "fact の種別"),
			fieldSchema("key", "string", "fact の一意キー"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind string `json:"kind"`
			Key  string `json:"key"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Key) == "" {
			return codex.ToolResponse{}, errors.New("kind and key are required")
		}
		if err := a.store.DeleteFact(ctx, input.Kind, input.Key); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("deleted"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.recent_summaries",
		Description: "保存済み summary を period 単位で読む",
		InputSchema: objectSchema(
			fieldSchema("period", "string", "daily, weekly, growth, wake など"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Period string `json:"period"`
			Limit  int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if strings.TrimSpace(input.Period) == "" {
			return codex.ToolResponse{}, errors.New("period is required")
		}
		if input.Limit <= 0 {
			input.Limit = 5
		}
		summaries, err := a.store.RecentSummaries(ctx, input.Period, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(summaries) == 0 {
			return textTool("no summaries"), nil
		}
		lines := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			lines = append(lines, fmt.Sprintf("- [%s] channel=%s %s", summary.CreatedAt.In(a.loc).Format(time.RFC3339), summary.ChannelID, truncateText(summary.Content, 240)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_channel_profiles",
		Description: "学習済みの channel profile を一覧する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(profiles) == 0 {
			return textTool("no channel profiles"), nil
		}
		lines := make([]string, 0, len(profiles))
		for _, profile := range profiles {
			lines = append(lines, fmt.Sprintf("- %s id=%s kind=%s reply=%.2f autonomy=%.2f cadence=%s", profile.Name, profile.ChannelID, profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel, profile.SummaryCadence))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.set_channel_profile",
		Description: "channel profile を更新して振る舞いの基準を整える",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("name", "string", "チャンネル名"),
			fieldSchema("kind", "string", "conversation, monologue, notifications など"),
			fieldSchema("reply_aggressiveness", "number", "0.0-1.0"),
			fieldSchema("autonomy_level", "number", "0.0-1.0"),
			fieldSchema("summary_cadence", "string", "daily, weekly など"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ChannelID           string  `json:"channel_id"`
			Name                string  `json:"name"`
			Kind                string  `json:"kind"`
			ReplyAggressiveness float64 `json:"reply_aggressiveness"`
			AutonomyLevel       float64 `json:"autonomy_level"`
			SummaryCadence      string  `json:"summary_cadence"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("channel_id is required")
		}

		profile, ok, err := a.store.GetChannelProfile(ctx, input.ChannelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			profile = memory.ChannelProfile{ChannelID: input.ChannelID}
		}
		if input.Name != "" {
			profile.Name = input.Name
		}
		if input.Kind != "" {
			profile.Kind = input.Kind
		}
		if input.ReplyAggressiveness > 0 {
			profile.ReplyAggressiveness = input.ReplyAggressiveness
		}
		if input.AutonomyLevel > 0 {
			profile.AutonomyLevel = input.AutonomyLevel
		}
		if input.SummaryCadence != "" {
			profile.SummaryCadence = input.SummaryCadence
		}
		if profile.Name == "" {
			profile.Name = input.ChannelID
		}
		if profile.Kind == "" {
			profile.Kind = "conversation"
		}
		if profile.ReplyAggressiveness == 0 {
			profile.ReplyAggressiveness = 0.75
		}
		if profile.AutonomyLevel == 0 {
			profile.AutonomyLevel = 0.55
		}
		if profile.SummaryCadence == "" {
			profile.SummaryCadence = "daily"
		}
		if err := a.store.UpsertChannelProfile(ctx, profile); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.channel_activity",
		Description: "最近のチャンネル活動量を俯瞰する",
		InputSchema: objectSchema(
			fieldSchema("since_hours", "integer", "何時間ぶん見るか"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			SinceHours int `json:"since_hours"`
			Limit      int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(activity) == 0 {
			return textTool("no recent activity"), nil
		}
		lines := make([]string, 0, len(activity))
		for _, item := range activity {
			lines = append(lines, fmt.Sprintf("- %s id=%s messages=%d last=%s", item.ChannelName, item.ChannelID, item.MessageCount, item.LastMessageAt.In(a.loc).Format(time.RFC3339)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.recent_owner_messages",
		Description: "オーナーの最近の発話を横断的に読み返す",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "特定チャンネルに絞る場合のチャンネル ID"),
			fieldSchema("query", "string", "検索語。省略時は時系列で新しいものを返す"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ChannelID string `json:"channel_id"`
			Query     string `json:"query"`
			Limit     int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 10
		}

		var messages []memory.Message
		if strings.TrimSpace(input.Query) == "" {
			var err error
			messages, err = a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, input.ChannelID, input.Limit)
			if err != nil {
				return codex.ToolResponse{}, err
			}
		} else {
			hits, err := a.store.SearchMessages(ctx, input.Query, max(input.Limit*4, 20))
			if err != nil {
				return codex.ToolResponse{}, err
			}
			for _, msg := range hits {
				if msg.AuthorID != a.cfg.Discord.OwnerUserID {
					continue
				}
				if input.ChannelID != "" && msg.ChannelID != input.ChannelID {
					continue
				}
				messages = append(messages, msg)
				if len(messages) >= input.Limit {
					break
				}
			}
		}

		if len(messages) == 0 {
			return textTool("no owner messages"), nil
		}
		lines := make([]string, 0, len(messages))
		for _, msg := range messages {
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", msg.CreatedAt.In(a.loc).Format(time.RFC3339), msg.ChannelName, truncateText(msg.Content, 220)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_open_loops",
		Description: "未解決の open loop を一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		loops, err := a.store.ListFacts(ctx, "open_loop", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(loops) == 0 {
			return textTool("no open loops"), nil
		}
		lines := make([]string, 0, len(loops))
		for _, loop := range loops {
			lines = append(lines, fmt.Sprintf("- %s: %s", loop.Key, loop.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_open_loop",
		Description: "未解決の問いや保留中の論点を open loop として保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "open loop の一意キー"),
			fieldSchema("value", "string", "保留内容"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Value) == "" {
			return codex.ToolResponse{}, errors.New("key and value are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            "open_loop",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.close_open_loop",
		Description: "open loop を解決済みにして閉じる",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "閉じる open loop のキー"),
			fieldSchema("resolution", "string", "解決内容。省略可"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Key             string `json:"key"`
			Resolution      string `json:"resolution"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" {
			return codex.ToolResponse{}, errors.New("key is required")
		}
		if err := a.store.DeleteFact(ctx, "open_loop", input.Key); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Resolution) != "" {
			if err := a.store.UpsertFact(ctx, memory.Fact{
				Kind:            "decision",
				Key:             "close/" + input.Key,
				Value:           input.Resolution,
				SourceMessageID: input.SourceMessageID,
			}); err != nil {
				return codex.ToolResponse{}, err
			}
		}
		return textTool("closed"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_reflection",
		Description: "会話や状況の振り返りを reflection として保存する",
		InputSchema: objectSchema(
			fieldSchema("content", "string", "振り返り内容"),
			fieldSchema("channel_id", "string", "関連チャンネル ID"),
			fieldSchema("period", "string", "summary period。省略時は reflection"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Content   string `json:"content"`
			ChannelID string `json:"channel_id"`
			Period    string `json:"period"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Content) == "" {
			return codex.ToolResponse{}, errors.New("content is required")
		}
		period := strings.TrimSpace(input.Period)
		if period == "" {
			period = "reflection"
		}
		now := time.Now().UTC()
		if err := a.store.SaveSummary(ctx, memory.Summary{
			Period:    period,
			ChannelID: input.ChannelID,
			Content:   input.Content,
			StartsAt:  now,
			EndsAt:    now,
			CreatedAt: now,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_growth_log",
		Description: "成長ログを保存する",
		InputSchema: objectSchema(
			fieldSchema("content", "string", "成長内容"),
			fieldSchema("channel_id", "string", "関連チャンネル ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Content   string `json:"content"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Content) == "" {
			return codex.ToolResponse{}, errors.New("content is required")
		}
		now := time.Now().UTC()
		if err := a.store.SaveSummary(ctx, memory.Summary{
			Period:    "growth",
			ChannelID: input.ChannelID,
			Content:   input.Content,
			StartsAt:  now,
			EndsAt:    now,
			CreatedAt: now,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_decision_log",
		Description: "判断や決定の履歴を decision log として保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "decision の一意キー"),
			fieldSchema("value", "string", "決定内容"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Value) == "" {
			return codex.ToolResponse{}, errors.New("key and value are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            "decision",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})
}

func (a *App) registerJobExtraTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "jobs.get",
		Description: "単一 job の状態を見る",
		InputSchema: objectSchema(fieldSchema("id", "string", "job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		job, ok, err := a.store.GetJob(ctx, input.ID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			return textTool("job not found"), nil
		}
		return textTool(formatJob(job)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.run_now",
		Description: "既存 job の次回実行を今に寄せる",
		InputSchema: objectSchema(fieldSchema("id", "string", "job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		job, ok, err := a.store.GetJob(ctx, input.ID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			return codex.ToolResponse{}, errors.New("job not found")
		}
		now := time.Now().UTC()
		if err := a.store.UpdateJobState(ctx, job.ID, jobs.StatePending, now, "", job.LastRunAt); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("scheduled for immediate run"), nil
	})

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
			input.Schedule = a.cfg.Behavior.ReleaseWatchInterval
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
		Description: "summary 系の定期 job を作る",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "daily_summary, weekly_review, growth_log"),
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
		if input.Kind != "daily_summary" && input.Kind != "weekly_review" && input.Kind != "growth_log" {
			return codex.ToolResponse{}, errors.New("unsupported summary kind")
		}
		if input.Schedule == "" {
			switch input.Kind {
			case "daily_summary", "growth_log":
				input.Schedule = "24h"
			case "weekly_review":
				input.Schedule = "168h"
			}
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
			input.Schedule = a.cfg.Behavior.ReleaseWatchInterval
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
}

func (a *App) registerDiscordExtraTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "discord.self_permissions",
		Description: "現在の bot 自身が指定チャンネルで持つ主要権限を確認する",
		InputSchema: objectSchema(fieldSchema("channel_id", "string", "確認対象チャンネル ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("channel_id is required")
		}
		snapshot, err := a.discord.SelfChannelPermissions(ctx, input.ChannelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("user_id=%s channel_id=%s raw=%d view_channel=%t send_messages=%t manage_channels=%t",
			snapshot.UserID,
			snapshot.ChannelID,
			snapshot.Raw,
			snapshot.ViewChannel,
			snapshot.SendMessages,
			snapshot.ManageChannels,
		)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.get_channel",
		Description: "単一チャンネルの詳細を取得する",
		InputSchema: objectSchema(fieldSchema("channel_id", "string", "対象チャンネル ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.GetChannel(ctx, input.ChannelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(formatChannel(channel)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.rename_channel",
		Description: "チャンネル名を変更する",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("name", "string", "新しいチャンネル名"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Name      string `json:"name"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.RenameChannel(ctx, input.ChannelID, sanitizeChannelName(input.Name))
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(formatChannel(channel)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.set_channel_topic",
		Description: "チャンネルの topic を変更する",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("topic", "string", "新しい topic"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Topic     string `json:"topic"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.SetChannelTopic(ctx, input.ChannelID, input.Topic)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(formatChannel(channel)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.describe_server",
		Description: "カテゴリ構造、channel profile、最近の活動量をまとめて俯瞰する",
		InputSchema: objectSchema(fieldSchema("since_hours", "integer", "最近の活動を見る時間幅")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			SinceHours int `json:"since_hours"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), 64)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeServer(channels, profiles, activity, a.loc)), nil
	})
}

func (a *App) registerWebTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "web.fetch_url",
		Description: "URL を取得して title と本文抜粋を読む",
		InputSchema: objectSchema(
			fieldSchema("url", "string", "取得対象 URL"),
			fieldSchema("max_chars", "integer", "本文の最大文字数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			URL      string `json:"url"`
			MaxChars int    `json:"max_chars"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.URL) == "" {
			return codex.ToolResponse{}, errors.New("url is required")
		}
		snapshot, err := a.fetchURLSnapshot(ctx, input.URL, input.MaxChars)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{
			fmt.Sprintf("title=%s", snapshot.Title),
			fmt.Sprintf("status=%d", snapshot.StatusCode),
			fmt.Sprintf("content_type=%s", snapshot.ContentType),
			fmt.Sprintf("final_url=%s", snapshot.FinalURL),
			fmt.Sprintf("text=%s", snapshot.Text),
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
}

func (a *App) registerMediaTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "media.load_attachments",
		Description: "画像 URL 群を会話コンテキストへ読み込み、スクリーンショットや画像添付を見られるようにする",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"urls": map[string]any{
					"type":        "array",
					"description": "画像 URL の配列",
					"items": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			URLs []string `json:"urls"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		imageInputs, notes := a.buildImageInputs(ctx, input.URLs)
		if len(imageInputs) == 0 {
			return codex.ToolResponse{}, errors.New("urls are required")
		}
		items := make([]codex.ToolContentItem, 0, len(imageInputs)+1)
		prefix := codex.ToolContentItem{
			Type: "inputText",
			Text: "loaded attachments:\n" + strings.Join(notes, "\n"),
		}
		items = append(items, prefix)
		for _, inputItem := range imageInputs {
			items = append(items, codex.ToolContentItem{
				Type:     "inputImage",
				ImageURL: inputItem.URL,
			})
		}
		return codex.ToolResponse{Success: true, ContentItems: items}, nil
	})
}

func formatJob(job jobs.Job) string {
	return fmt.Sprintf("id=%s kind=%s state=%s channel=%s schedule=%s next=%s last_error=%s payload=%s",
		job.ID,
		job.Kind,
		job.State,
		job.ChannelID,
		job.ScheduleExpr,
		job.NextRunAt.Format(time.RFC3339),
		job.LastError,
		formatMap(job.Payload),
	)
}

func formatChannel(channel discordsvc.Channel) string {
	return fmt.Sprintf("id=%s name=%s type=%d parent=%s position=%d topic=%s",
		channel.ID,
		channel.Name,
		channel.Type,
		channel.ParentID,
		channel.Position,
		channel.Topic,
	)
}

func describeServer(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, loc *time.Location) string {
	categories := map[string]discordsvc.Channel{}
	children := map[string][]discordsvc.Channel{}
	var roots []discordsvc.Channel
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			categories[channel.ID] = channel
			continue
		}
		if channel.ParentID == "" {
			roots = append(roots, channel)
			continue
		}
		children[channel.ParentID] = append(children[channel.ParentID], channel)
	}

	activityByChannel := map[string]memory.ChannelActivity{}
	for _, item := range activity {
		activityByChannel[item.ChannelID] = item
	}
	profileByChannel := map[string]memory.ChannelProfile{}
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}

	lines := []string{"categories:"}
	for _, category := range channels {
		if category.Type != discordgo.ChannelTypeGuildCategory {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s id=%s", category.Name, category.ID))
		for _, child := range children[category.ID] {
			lines = append(lines, "- "+describeServerChannel(child, profileByChannel[child.ID], activityByChannel[child.ID], loc))
		}
	}
	lines = append(lines, "root_channels:")
	for _, channel := range roots {
		lines = append(lines, "- "+describeServerChannel(channel, profileByChannel[channel.ID], activityByChannel[channel.ID], loc))
	}
	lines = append(lines, "known_profiles:")
	if len(profiles) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, profile := range profiles {
			lines = append(lines, fmt.Sprintf("- %s id=%s kind=%s reply=%.2f autonomy=%.2f cadence=%s", profile.Name, profile.ChannelID, profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel, profile.SummaryCadence))
		}
	}
	return strings.Join(lines, "\n")
}

func describeServerChannel(channel discordsvc.Channel, profile memory.ChannelProfile, activity memory.ChannelActivity, loc *time.Location) string {
	parts := []string{
		fmt.Sprintf("%s id=%s type=%d", channel.Name, channel.ID, channel.Type),
	}
	if channel.Topic != "" {
		parts = append(parts, "topic="+truncateText(channel.Topic, 80))
	}
	if !activity.LastMessageAt.IsZero() {
		parts = append(parts, fmt.Sprintf("messages=%d last=%s", activity.MessageCount, activity.LastMessageAt.In(loc).Format(time.RFC3339)))
	}
	if profile.Kind != "" {
		parts = append(parts, fmt.Sprintf("profile=%s reply=%.2f autonomy=%.2f", profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel))
	}
	return strings.Join(parts, " | ")
}

func formatMap(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
