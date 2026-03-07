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
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func (a *App) registerDiscordAutonomyTools(registry *codex.ToolRegistry) {
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
		snapshot := describeServer(channels, profiles, activity, loc)
		now := time.Now().UTC()
		content := formatSpaceSnapshotContent(strings.TrimSpace(input.Label), input.SinceHours, snapshot)
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

func formatSpaceSnapshotContent(label string, sinceHours int, snapshot string) string {
	if strings.TrimSpace(label) == "" {
		label = "snapshot"
	}
	lines := []string{
		fmt.Sprintf("snapshot label: %s", strings.TrimSpace(label)),
		fmt.Sprintf("since_hours: %d", sinceHours),
		strings.TrimSpace(snapshot),
	}
	return strings.Join(lines, "\n")
}

func firstNonEmptyLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func diffSpaceSnapshotContents(older string, newer string) string {
	olderLines := snapshotComparableLines(older)
	newerLines := snapshotComparableLines(newer)

	olderSet := make(map[string]struct{}, len(olderLines))
	for _, line := range olderLines {
		olderSet[line] = struct{}{}
	}
	newerSet := make(map[string]struct{}, len(newerLines))
	for _, line := range newerLines {
		newerSet[line] = struct{}{}
	}

	var added []string
	for _, line := range newerLines {
		if _, ok := olderSet[line]; ok {
			continue
		}
		added = append(added, "+ "+line)
	}
	var removed []string
	for _, line := range olderLines {
		if _, ok := newerSet[line]; ok {
			continue
		}
		removed = append(removed, "- "+line)
	}

	lines := []string{
		fmt.Sprintf("newer: %s", firstNonEmptyLine(newer)),
		fmt.Sprintf("older: %s", firstNonEmptyLine(older)),
	}
	if len(added) == 0 && len(removed) == 0 {
		lines = append(lines, "no line-level diff")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "added:")
	if len(added) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, added...)
	}
	lines = append(lines, "removed:")
	if len(removed) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, removed...)
	}
	return strings.Join(lines, "\n")
}

func snapshotComparableLines(content string) []string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "snapshot label:") || strings.HasPrefix(line, "since_hours:") {
			continue
		}
		out = append(out, line)
	}
	return out
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

func describeCategoryMap(channels []discordsvc.Channel) string {
	categories := make(map[string]discordsvc.Channel)
	children := make(map[string][]discordsvc.Channel)
	rootText := make([]discordsvc.Channel, 0)
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			categories[channel.ID] = channel
			continue
		}
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if strings.TrimSpace(channel.ParentID) == "" {
			rootText = append(rootText, channel)
			continue
		}
		children[channel.ParentID] = append(children[channel.ParentID], channel)
	}

	categoryIDs := make([]string, 0, len(categories))
	for id := range categories {
		categoryIDs = append(categoryIDs, id)
	}
	slices.Sort(categoryIDs)

	lines := []string{"category map:"}
	for _, id := range categoryIDs {
		category := categories[id]
		lines = append(lines, fmt.Sprintf("- %s (%s)", category.Name, category.ID))
		items := children[id]
		if len(items) == 0 {
			lines = append(lines, "  - empty")
			continue
		}
		slices.SortFunc(items, func(left discordsvc.Channel, right discordsvc.Channel) int {
			return strings.Compare(left.Name, right.Name)
		})
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("  - %s (%s)", item.Name, item.ID))
		}
	}
	if len(rootText) == 0 {
		lines = append(lines, "root text channels: none")
	} else {
		slices.SortFunc(rootText, func(left discordsvc.Channel, right discordsvc.Channel) int {
			return strings.Compare(left.Name, right.Name)
		})
		names := make([]string, 0, len(rootText))
		for _, channel := range rootText {
			names = append(names, fmt.Sprintf("%s (%s)", channel.Name, channel.ID))
		}
		lines = append(lines, "root text channels: "+strings.Join(names, ", "))
	}
	return strings.Join(lines, "\n")
}

func describeOrphanChannels(channels []discordsvc.Channel) string {
	categoryChildren := make(map[string]int)
	categoryNames := make(map[string]string)
	rootText := make([]string, 0)
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			categoryNames[channel.ID] = channel.Name
			continue
		}
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if strings.TrimSpace(channel.ParentID) == "" {
			rootText = append(rootText, fmt.Sprintf("%s (%s)", channel.Name, channel.ID))
			continue
		}
		categoryChildren[channel.ParentID]++
	}
	emptyCategories := make([]string, 0)
	for id, name := range categoryNames {
		if categoryChildren[id] == 0 {
			emptyCategories = append(emptyCategories, fmt.Sprintf("%s (%s)", name, id))
		}
	}
	slices.Sort(rootText)
	slices.Sort(emptyCategories)
	lines := []string{}
	if len(rootText) == 0 {
		lines = append(lines, "root text channels: none")
	} else {
		lines = append(lines, "root text channels: "+strings.Join(rootText, ", "))
	}
	if len(emptyCategories) == 0 {
		lines = append(lines, "empty categories: none")
	} else {
		lines = append(lines, "empty categories: "+strings.Join(emptyCategories, ", "))
	}
	return strings.Join(lines, "\n")
}

func suggestChannelProfiles(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, sinceHours int) string {
	profileByChannel := make(map[string]memory.ChannelProfile, len(profiles))
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}
	activityByChannel := make(map[string]memory.ChannelActivity, len(activity))
	for _, item := range activity {
		activityByChannel[item.ChannelID] = item
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("profile suggestions over last %dh:", sinceHours))
	for _, channel := range channels {
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if _, ok := profileByChannel[channel.ID]; ok {
			continue
		}
		item, active := activityByChannel[channel.ID]
		suggestion := "conversation"
		replyAgg := 0.75
		autonomy := 0.55
		cadence := "daily"
		reason := "default starting point"

		switch {
		case active && item.MessageCount >= 12 && channel.ParentID == "":
			suggestion = "conversation"
			replyAgg = 0.8
			autonomy = 0.6
			reason = "active and rooted in the main flow"
		case active && item.MessageCount >= 6:
			suggestion = "conversation"
			replyAgg = 0.72
			autonomy = 0.58
			reason = "active enough to stay responsive"
		case !active && strings.TrimSpace(channel.Topic) != "":
			suggestion = "reference"
			replyAgg = 0.2
			autonomy = 0.35
			cadence = "weekly"
			reason = "quiet with an explicit topic, so reference/archive style may fit"
		case !active:
			suggestion = "monologue"
			replyAgg = 0.2
			autonomy = 0.75
			cadence = "weekly"
			reason = "quiet space without recent traffic"
		}

		lines = append(lines, fmt.Sprintf("- %s (%s): suggest kind=%s reply=%.2f autonomy=%.2f cadence=%s reason=%s", channel.Name, channel.ID, suggestion, replyAgg, autonomy, cadence, reason))
	}
	if len(lines) == 1 {
		lines = append(lines, "no unprofiled text channels")
	}
	return strings.Join(lines, "\n")
}
