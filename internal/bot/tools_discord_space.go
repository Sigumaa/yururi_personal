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

func (a *App) registerDiscordSpaceManagementTools(registry *codex.ToolRegistry) {
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
