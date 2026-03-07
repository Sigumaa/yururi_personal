package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/bwmarrin/discordgo"
)

func (a *App) registerDiscordReadWriteTools(registry *codex.ToolRegistry) {
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
		Name:        "discord.find_channels",
		Description: "チャンネル名、topic、親カテゴリ、種別でチャンネルを探す",
		InputSchema: objectSchema(
			fieldSchema("query", "string", "チャンネル名または topic の部分一致。省略可"),
			fieldSchema("parent_channel_id", "string", "親カテゴリ ID。省略可"),
			fieldSchema("kind", "string", "text または category。省略可"),
			fieldSchema("limit", "integer", "返す件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Query           string `json:"query"`
			ParentChannelID string `json:"parent_channel_id"`
			Kind            string `json:"kind"`
			Limit           int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 12
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		query := strings.ToLower(strings.TrimSpace(input.Query))
		kind := strings.ToLower(strings.TrimSpace(input.Kind))
		lines := make([]string, 0, input.Limit)
		for _, channel := range channels {
			if strings.TrimSpace(input.ParentChannelID) != "" && channel.ParentID != input.ParentChannelID {
				continue
			}
			if query != "" {
				haystack := strings.ToLower(channel.Name + "\n" + channel.Topic)
				if !strings.Contains(haystack, query) {
					continue
				}
			}
			switch kind {
			case "text":
				if channel.Type != discordgo.ChannelTypeGuildText {
					continue
				}
			case "category":
				if channel.Type != discordgo.ChannelTypeGuildCategory {
					continue
				}
			}
			lines = append(lines, "- "+formatChannel(channel))
			if len(lines) >= input.Limit {
				break
			}
		}
		if len(lines) == 0 {
			return textTool("no matching channels"), nil
		}
		return textTool(strings.Join(lines, "\n")), nil
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
		Name:        "discord.ensure_space",
		Description: "カテゴリと配下チャンネル群を一度に整備する",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"category_name": map[string]any{"type": "string", "description": "作成または再利用するカテゴリ名"},
				"channels": map[string]any{
					"type":        "array",
					"description": "作成または再利用するチャンネル一覧",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"name":  map[string]any{"type": "string"},
							"topic": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			CategoryName string `json:"category_name"`
			Channels     []struct {
				Name  string `json:"name"`
				Topic string `json:"topic"`
			} `json:"channels"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.CategoryName) == "" {
			return codex.ToolResponse{}, errors.New("category_name is required")
		}
		category, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, input.CategoryName)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{fmt.Sprintf("category %s (%s)", category.Name, category.ID)}
		for _, item := range input.Channels {
			if strings.TrimSpace(item.Name) == "" {
				continue
			}
			channel, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
				Name:     sanitizeChannelName(item.Name),
				Topic:    item.Topic,
				ParentID: category.ID,
			})
			if err != nil {
				return codex.ToolResponse{}, err
			}
			lines = append(lines, fmt.Sprintf("- %s (%s)", channel.Name, channel.ID))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.move_channels_batch",
		Description: "複数チャンネルを一括で同じカテゴリへ移動する",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"parent_channel_id": map[string]any{"type": "string"},
				"channel_ids": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ParentChannelID string   `json:"parent_channel_id"`
			ChannelIDs      []string `json:"channel_ids"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ParentChannelID) == "" || len(input.ChannelIDs) == 0 {
			return codex.ToolResponse{}, errors.New("parent_channel_id and channel_ids are required")
		}
		lines := make([]string, 0, len(input.ChannelIDs))
		for _, channelID := range input.ChannelIDs {
			if strings.TrimSpace(channelID) == "" {
				continue
			}
			if err := a.discord.MoveChannel(ctx, channelID, input.ParentChannelID); err != nil {
				return codex.ToolResponse{}, err
			}
			lines = append(lines, fmt.Sprintf("- moved %s", channelID))
		}
		if len(lines) == 0 {
			return textTool("no channels moved"), nil
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.archive_channels",
		Description: "チャンネル群を archive カテゴリへまとめて移動し、必要なら名前に prefix を付ける",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"channel_ids": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
				},
				"archive_category_name": map[string]any{"type": "string"},
				"rename_prefix":         map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelIDs          []string `json:"channel_ids"`
			ArchiveCategoryName string   `json:"archive_category_name"`
			RenamePrefix        string   `json:"rename_prefix"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if len(input.ChannelIDs) == 0 {
			return codex.ToolResponse{}, errors.New("channel_ids are required")
		}
		if strings.TrimSpace(input.ArchiveCategoryName) == "" {
			input.ArchiveCategoryName = "archive"
		}
		category, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, input.ArchiveCategoryName)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{fmt.Sprintf("archive %s (%s)", category.Name, category.ID)}
		for _, channelID := range input.ChannelIDs {
			channel, err := a.discord.GetChannel(ctx, channelID)
			if err != nil {
				return codex.ToolResponse{}, err
			}
			if err := a.discord.MoveChannel(ctx, channelID, category.ID); err != nil {
				return codex.ToolResponse{}, err
			}
			if strings.TrimSpace(input.RenamePrefix) != "" && !strings.HasPrefix(channel.Name, input.RenamePrefix) {
				channel, err = a.discord.RenameChannel(ctx, channelID, sanitizeChannelName(input.RenamePrefix+"-"+channel.Name))
				if err != nil {
					return codex.ToolResponse{}, err
				}
			}
			lines = append(lines, fmt.Sprintf("- archived %s (%s)", channel.Name, channel.ID))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
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
