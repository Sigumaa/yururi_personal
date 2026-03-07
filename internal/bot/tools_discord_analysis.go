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

func (a *App) registerDiscordAnalysisTools(registry *codex.ToolRegistry) {
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

func describeServer(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, loc *time.Location) string {
	children := map[string][]discordsvc.Channel{}
	var roots []discordsvc.Channel
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
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
	parts := []string{fmt.Sprintf("%s id=%s type=%d", channel.Name, channel.ID, channel.Type)}
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
