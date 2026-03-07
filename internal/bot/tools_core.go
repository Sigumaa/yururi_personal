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
)

func (a *App) registerTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "tools.list",
		Description: "使える tool を一覧する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		specs := registry.Specs()
		if len(specs) == 0 {
			return textTool("no tools"), nil
		}
		lines := make([]string, 0, len(specs))
		for _, spec := range specs {
			lines = append(lines, fmt.Sprintf("- %s: %s", toolAlias(spec.Name), spec.Description))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.search",
		Description: "保存済みメッセージと fact から関連情報を検索する",
		InputSchema: objectSchema(
			fieldSchema("query", "string", "検索語"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit == 0 {
			input.Limit = 5
		}
		messages, err := a.store.SearchMessages(ctx, input.Query, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		facts, err := a.store.SearchFacts(ctx, input.Query, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{"messages:"}
		for _, msg := range messages {
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", msg.ChannelName, msg.AuthorName, msg.Content))
		}
		lines = append(lines, "facts:")
		for _, fact := range facts {
			lines = append(lines, fmt.Sprintf("- %s/%s: %s", fact.Kind, fact.Key, fact.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_fact",
		Description: "長期記憶として fact を 1 件保存または更新する",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "fact の種別"),
			fieldSchema("key", "string", "fact の一意キー"),
			fieldSchema("value", "string", "保存する内容"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind            string `json:"kind"`
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Key) == "" {
			return codex.ToolResponse{}, errors.New("kind and key are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            input.Kind,
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})

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

	registry.Register(codex.ToolSpec{
		Name:        "discord.list_channels",
		Description: "サーバー内のチャンネル一覧を取得する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(channels))
		for _, channel := range channels {
			lines = append(lines, fmt.Sprintf("- %s id=%s parent=%s type=%d", channel.Name, channel.ID, channel.ParentID, channel.Type))
		}
		if len(lines) == 0 {
			lines = append(lines, "no channels")
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.read_recent_messages",
		Description: "指定チャンネルの直近メッセージを読む",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Limit     int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit == 0 {
			input.Limit = 10
		}
		messages, err := a.discord.RecentMessages(ctx, input.ChannelID, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, len(messages))
		for _, msg := range messages {
			lines = append(lines, fmt.Sprintf("- %s: %s", msg.AuthorName, msg.Content))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.get_member_presence",
		Description: "ユーザーの現在の presence と activity を取得する",
		InputSchema: objectSchema(fieldSchema("user_id", "string", "対象ユーザー ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			UserID string `json:"user_id"`
		}
		_ = json.Unmarshal(raw, &input)
		presence, err := a.discord.CurrentPresence(ctx, a.cfg.Discord.GuildID, input.UserID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("status=%s activities=%s", presence.Status, strings.Join(presence.Activities, ", "))), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.send_message",
		Description: "指定チャンネルへメッセージを送る。進捗共有や途中経過の連投にも使える",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "送信先チャンネル ID"),
			fieldSchema("content", "string", "送信内容"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Content   string `json:"content"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		messageID, err := a.discord.SendMessage(ctx, input.ChannelID, input.Content)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("sent %s; まだ残りの作業があるなら、この turn の中で続けて進めてよい", messageID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.create_category",
		Description: "カテゴリチャンネルを作成する",
		InputSchema: objectSchema(fieldSchema("name", "string", "カテゴリ名")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, input.Name)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("created %s (%s)", channel.Name, channel.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.create_channel",
		Description: "テキストチャンネルを作成する。parent_channel_id を省略するとルート直下に作る",
		InputSchema: objectSchema(
			fieldSchema("name", "string", "チャンネル名"),
			fieldSchema("topic", "string", "トピック"),
			fieldSchema("parent_channel_id", "string", "親カテゴリ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Name            string `json:"name"`
			Topic           string `json:"topic"`
			ParentChannelID string `json:"parent_channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
			Name:     sanitizeChannelName(input.Name),
			Topic:    input.Topic,
			ParentID: input.ParentChannelID,
		})
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("created %s (%s)", channel.Name, channel.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.move_channel",
		Description: "チャンネルを別カテゴリへ移動する",
		InputSchema: objectSchema(
			fieldSchema("target_channel_id", "string", "移動対象チャンネル ID"),
			fieldSchema("parent_channel_id", "string", "移動先カテゴリ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			TargetChannelID string `json:"target_channel_id"`
			ParentChannelID string `json:"parent_channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if err := a.discord.MoveChannel(ctx, input.TargetChannelID, input.ParentChannelID); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})

	a.registerExtraTools(registry)
}
