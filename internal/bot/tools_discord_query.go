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

func (a *App) registerDiscordQueryTools(registry *codex.ToolRegistry) {
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
