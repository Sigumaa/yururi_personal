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
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

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

	registry.Register(codex.ToolSpec{
		Name:        "discord.describe_idle_channels",
		Description: "最近使われていないチャンネルを活動量と profile つきで俯瞰する",
		InputSchema: objectSchema(
			fieldSchema("since_hours", "integer", "何時間動きがなければ idle とみなすか"),
			fieldSchema("limit", "integer", "返す件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			SinceHours int `json:"since_hours"`
			Limit      int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		if input.Limit <= 0 {
			input.Limit = 12
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), 256)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeIdleChannels(channels, profiles, activity, input.Limit)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.describe_space_candidates",
		Description: "空間整理の候補を root/idle/profile 観点で俯瞰する",
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
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), 256)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeSpaceCandidates(channels, profiles, activity, a.loc)), nil
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

func describeIdleChannels(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, limit int) string {
	active := map[string]bool{}
	for _, item := range activity {
		active[item.ChannelID] = true
	}
	profileByChannel := map[string]memory.ChannelProfile{}
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}

	lines := []string{"idle_channels:"}
	count := 0
	for _, channel := range channels {
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if active[channel.ID] {
			continue
		}
		profile := profileByChannel[channel.ID]
		line := fmt.Sprintf("- %s id=%s parent=%s", channel.Name, channel.ID, channel.ParentID)
		if profile.Kind != "" {
			line += fmt.Sprintf(" profile=%s reply=%.2f autonomy=%.2f", profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel)
		}
		lines = append(lines, line)
		count++
		if count >= limit {
			break
		}
	}
	if count == 0 {
		lines = append(lines, "- none")
	}
	return strings.Join(lines, "\n")
}

func describeSpaceCandidates(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, loc *time.Location) string {
	categoryNames := map[string]string{}
	childrenCount := map[string]int{}
	profileByChannel := map[string]memory.ChannelProfile{}
	activityByChannel := map[string]memory.ChannelActivity{}
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}
	for _, item := range activity {
		activityByChannel[item.ChannelID] = item
	}

	var activeRoots []string
	var missingProfiles []string
	var quietProfiled []string
	var emptyCategories []string

	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			categoryNames[channel.ID] = channel.Name
			continue
		}
		if channel.ParentID != "" {
			childrenCount[channel.ParentID]++
		}
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if channel.ParentID == "" {
			if item, ok := activityByChannel[channel.ID]; ok {
				activeRoots = append(activeRoots, fmt.Sprintf("- %s id=%s messages=%d last=%s", channel.Name, channel.ID, item.MessageCount, item.LastMessageAt.In(loc).Format(time.RFC3339)))
			}
		}
		if _, ok := profileByChannel[channel.ID]; !ok {
			parentName := categoryNames[channel.ParentID]
			if parentName == "" {
				parentName = "root"
			}
			missingProfiles = append(missingProfiles, fmt.Sprintf("- %s id=%s parent=%s", channel.Name, channel.ID, parentName))
			continue
		}
		if _, ok := activityByChannel[channel.ID]; !ok {
			profile := profileByChannel[channel.ID]
			quietProfiled = append(quietProfiled, fmt.Sprintf("- %s id=%s profile=%s cadence=%s", channel.Name, channel.ID, profile.Kind, profile.SummaryCadence))
		}
	}
	for categoryID, name := range categoryNames {
		if childrenCount[categoryID] == 0 {
			emptyCategories = append(emptyCategories, fmt.Sprintf("- %s id=%s", name, categoryID))
		}
	}

	lines := []string{"active_root_channels:"}
	if len(activeRoots) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, activeRoots...)
	}
	lines = append(lines, "channels_missing_profile:")
	if len(missingProfiles) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, missingProfiles...)
	}
	lines = append(lines, "quiet_profiled_channels:")
	if len(quietProfiled) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, quietProfiled...)
	}
	lines = append(lines, "empty_categories:")
	if len(emptyCategories) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, emptyCategories...)
	}
	return strings.Join(lines, "\n")
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
