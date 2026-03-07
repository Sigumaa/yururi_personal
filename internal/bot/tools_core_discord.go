package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
)

func (a *App) registerCoreDiscordTools(registry *codex.ToolRegistry) {
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
}
