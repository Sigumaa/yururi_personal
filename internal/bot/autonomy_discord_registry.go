package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (a *App) registerDiscordAutonomyReadTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "discord.describe_category_map",
		Description: "カテゴリごとの配下チャンネル構造を俯瞰する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeCategoryMap(channels)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.find_orphan_channels",
		Description: "親カテゴリのないテキストチャンネルや空のカテゴリを探す",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeOrphanChannels(channels)), nil
	})

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

	registry.Register(codex.ToolSpec{
		Name:        "discord.suggest_channel_profiles",
		Description: "最近の活動量と channel 情報から、channel profile の候補を提案する",
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
		return textTool(suggestChannelProfiles(channels, profiles, activity, input.SinceHours)), nil
	})
}

func (a *App) registerDiscordAutonomySnapshotTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "discord.capture_space_snapshot",
		Description: "現在のサーバー構造と最近の活動を space snapshot として保存する",
		InputSchema: objectSchema(
			fieldSchema("label", "string", "snapshot の短いラベル。省略可"),
			fieldSchema("since_hours", "integer", "最近の活動を見る時間幅"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Label      string `json:"label"`
			SinceHours int    `json:"since_hours"`
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
		loc := a.loc
		if loc == nil {
			loc = time.UTC
		}
		now := time.Now().UTC()
		content := formatSpaceSnapshotContent(strings.TrimSpace(input.Label), input.SinceHours, describeServer(channels, profiles, activity, loc))
		if err := a.store.SaveSummary(ctx, memory.Summary{
			Period:    "space_snapshot",
			ChannelID: "",
			Content:   content,
			StartsAt:  now,
			EndsAt:    now,
			CreatedAt: now,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("saved space snapshot label=%s", strings.TrimSpace(input.Label))), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.recent_space_snapshots",
		Description: "保存済みの space snapshot を新しい順に一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 5
		}
		snapshots, err := a.store.RecentSummaries(ctx, "space_snapshot", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(snapshots) == 0 {
			return textTool("no space snapshots"), nil
		}
		lines := make([]string, 0, len(snapshots))
		for _, item := range snapshots {
			lines = append(lines, fmt.Sprintf("- [%s] %s", item.CreatedAt.In(a.loc).Format(time.RFC3339), firstNonEmptyLine(item.Content)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.diff_recent_space_snapshots",
		Description: "直近 2 つの space snapshot の差分を簡潔に出す",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		snapshots, err := a.store.RecentSummaries(ctx, "space_snapshot", 2)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(snapshots) < 2 {
			return textTool("not enough space snapshots"), nil
		}
		diff := diffSpaceSnapshotContents(snapshots[1].Content, snapshots[0].Content)
		if strings.TrimSpace(diff) == "" {
			return textTool("no space snapshot diff"), nil
		}
		return textTool(diff), nil
	})
}
