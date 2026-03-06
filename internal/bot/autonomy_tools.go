package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func (a *App) registerAutonomyTools(registry *codex.ToolRegistry) {
	a.registerMemoryAutonomyTools(registry)
	a.registerJobAutonomyTools(registry)
	a.registerDiscordAutonomyTools(registry)
}

func (a *App) registerMemoryAutonomyTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.search_messages",
		Description: "会話履歴を横断検索して、関連する発話を拾う",
		InputSchema: objectSchema(
			fieldSchema("query", "string", "検索語"),
			fieldSchema("channel_id", "string", "特定チャンネルに絞る場合のチャンネル ID"),
			fieldSchema("author_id", "string", "特定ユーザーに絞る場合のユーザー ID"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Query     string `json:"query"`
			ChannelID string `json:"channel_id"`
			AuthorID  string `json:"author_id"`
			Limit     int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Query) == "" {
			return codex.ToolResponse{}, errors.New("query is required")
		}
		if input.Limit <= 0 {
			input.Limit = 10
		}
		messages, err := a.store.SearchMessages(ctx, input.Query, max(input.Limit*4, 20))
		if err != nil {
			return codex.ToolResponse{}, err
		}
		filtered := make([]memory.Message, 0, input.Limit)
		for _, msg := range messages {
			if input.ChannelID != "" && msg.ChannelID != input.ChannelID {
				continue
			}
			if input.AuthorID != "" && msg.AuthorID != input.AuthorID {
				continue
			}
			filtered = append(filtered, msg)
			if len(filtered) >= input.Limit {
				break
			}
		}
		if len(filtered) == 0 {
			return textTool("no matching messages"), nil
		}
		lines := make([]string, 0, len(filtered))
		for _, msg := range filtered {
			lines = append(lines, fmt.Sprintf("- [%s] %s/%s: %s", msg.CreatedAt.In(a.loc).Format(time.RFC3339), msg.ChannelName, msg.AuthorName, truncateText(msg.Content, 220)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_automation_candidates",
		Description: "自動化したい反復作業の候補を一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		candidates, err := a.store.ListFacts(ctx, "automation_candidate", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(candidates) == 0 {
			return textTool("no automation candidates"), nil
		}
		lines := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			lines = append(lines, fmt.Sprintf("- %s: %s", candidate.Key, candidate.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_automation_candidate",
		Description: "反復している依頼や自動化候補を記録する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "候補の一意キー"),
			fieldSchema("value", "string", "候補の説明"),
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
			Kind:            "automation_candidate",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})
}

func (a *App) registerJobAutonomyTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_review",
		Description: "open loop review や channel curation の継続 job を作る",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "open_loop_review または channel_curation"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration。省略時は kind ごとの既定値"),
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
		switch input.Kind {
		case "open_loop_review":
			if input.Schedule == "" {
				input.Schedule = "72h"
			}
		case "channel_curation":
			if input.Schedule == "" {
				input.Schedule = "168h"
			}
		default:
			return codex.ToolResponse{}, errors.New("unsupported review kind")
		}
		job := jobs.NewJob(jobID(strings.ReplaceAll(input.Kind, "_", "-")), input.Kind, input.Kind, input.ChannelID, input.Schedule, nil)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})
}

func (a *App) registerDiscordAutonomyTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "discord.find_stale_channels",
		Description: "最近動きの少ないテキストチャンネルを探す",
		InputSchema: objectSchema(fieldSchema("since_hours", "integer", "何時間ぶんを stale 判定に使うか")),
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
		stale := findStaleTextChannels(channels, activity)
		if len(stale) == 0 {
			return textTool("no stale channels"), nil
		}
		profileByChannel := make(map[string]memory.ChannelProfile, len(profiles))
		for _, profile := range profiles {
			profileByChannel[profile.ChannelID] = profile
		}
		lines := make([]string, 0, len(stale))
		for _, channel := range stale {
			line := fmt.Sprintf("- %s id=%s parent=%s", channel.Name, channel.ID, channel.ParentID)
			if profile, ok := profileByChannel[channel.ID]; ok {
				line += fmt.Sprintf(" profile=%s autonomy=%.2f", profile.Kind, profile.AutonomyLevel)
			}
			lines = append(lines, line)
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.plan_space_refresh",
		Description: "活動量とプロフィールから空間整理の観点をまとめる",
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
			activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), 256)
			if err != nil {
				return codex.ToolResponse{}, err
			}
			return textTool(planSpaceRefresh(channels, profiles, activity, input.SinceHours)), nil
		})
}

func findStaleTextChannels(channels []discordsvc.Channel, activity []memory.ChannelActivity) []discordsvc.Channel {
	active := make(map[string]struct{}, len(activity))
	for _, item := range activity {
		active[item.ChannelID] = struct{}{}
	}
	var stale []discordsvc.Channel
	for _, channel := range channels {
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if _, ok := active[channel.ID]; ok {
			continue
		}
		stale = append(stale, channel)
	}
	slices.SortFunc(stale, func(left discordsvc.Channel, right discordsvc.Channel) int {
		switch {
		case left.ParentID < right.ParentID:
			return -1
		case left.ParentID > right.ParentID:
			return 1
		case left.Name < right.Name:
			return -1
		case left.Name > right.Name:
			return 1
		default:
			return 0
		}
	})
	return stale
}

func planSpaceRefresh(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, sinceHours int) string {
	profileByChannel := make(map[string]memory.ChannelProfile, len(profiles))
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}
	categoryChildren := map[string]int{}
	rootTextChannels := []string{}
	unprofiled := []string{}
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			continue
		}
		if channel.ParentID != "" {
			categoryChildren[channel.ParentID]++
		}
		if channel.Type == discordgo.ChannelTypeGuildText && channel.ParentID == "" {
			rootTextChannels = append(rootTextChannels, channel.Name)
		}
		if channel.Type == discordgo.ChannelTypeGuildText {
			if _, ok := profileByChannel[channel.ID]; !ok {
				unprofiled = append(unprofiled, channel.Name)
			}
		}
	}
	stale := findStaleTextChannels(channels, activity)
	lonelyCategories := []string{}
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory && categoryChildren[channel.ID] == 0 {
			lonelyCategories = append(lonelyCategories, channel.Name)
		}
	}
	lines := []string{
		fmt.Sprintf("space refresh view over last %dh", sinceHours),
	}
	if len(rootTextChannels) == 0 {
		lines = append(lines, "- root text channels: none")
	} else {
		lines = append(lines, "- root text channels: "+strings.Join(rootTextChannels, ", "))
	}
	if len(unprofiled) == 0 {
		lines = append(lines, "- unprofiled channels: none")
	} else {
		lines = append(lines, "- unprofiled channels: "+strings.Join(unprofiled, ", "))
	}
	if len(lonelyCategories) == 0 {
		lines = append(lines, "- empty categories: none")
	} else {
		lines = append(lines, "- empty categories: "+strings.Join(lonelyCategories, ", "))
	}
	if len(stale) == 0 {
		lines = append(lines, "- stale channels: none")
	} else {
		names := make([]string, 0, len(stale))
		for _, channel := range stale {
			names = append(names, channel.Name)
		}
		lines = append(lines, "- stale channels: "+strings.Join(names, ", "))
	}
	lines = append(lines, "suggestions:")
	switch {
	case len(rootTextChannels) >= 4:
		lines = append(lines, "- root 直下のテキストチャンネルが増えているので、用途ごとにカテゴリをまとめる余地があります。")
	default:
		lines = append(lines, "- ルート直下はまだ暴れていません。必要が出たところから整える形で十分です。")
	}
	if len(stale) > 0 {
		lines = append(lines, "- stale な場所は、topic 更新や archive 候補の提案先として見られます。")
	}
	if len(unprofiled) > 0 {
		lines = append(lines, "- 振る舞いが定まっていないチャンネルは profile を付けると自律性が安定します。")
	}
	if len(lonelyCategories) > 0 {
		lines = append(lines, "- 空のカテゴリは育てるか畳むかを後で判断しやすい状態です。")
	}
	return strings.Join(lines, "\n")
}
