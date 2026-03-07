package space

import (
	"fmt"
	"slices"
	"strings"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func FormatSpaceSnapshotContent(label string, sinceHours int, snapshot string) string {
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

func FirstNonEmptyLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func DiffRecentSnapshots(older string, newer string) string {
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
		fmt.Sprintf("newer: %s", FirstNonEmptyLine(newer)),
		fmt.Sprintf("older: %s", FirstNonEmptyLine(older)),
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

func DescribeServer(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, loc *time.Location) string {
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

func DescribeIdleChannels(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, limit int) string {
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

func DescribeSpaceCandidates(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, loc *time.Location) string {
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

func FindStaleTextChannels(channels []discordsvc.Channel, activity []memory.ChannelActivity) []discordsvc.Channel {
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

func PlanRefresh(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, sinceHours int) string {
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
	stale := FindStaleTextChannels(channels, activity)
	lonelyCategories := []string{}
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory && categoryChildren[channel.ID] == 0 {
			lonelyCategories = append(lonelyCategories, channel.Name)
		}
	}
	lines := []string{fmt.Sprintf("space refresh view over last %dh", sinceHours)}
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
	if len(rootTextChannels) >= 4 {
		lines = append(lines, "- root 直下のテキストチャンネルが増えているので、用途ごとにカテゴリをまとめる余地があります。")
	} else {
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

func DescribeCategoryMap(channels []discordsvc.Channel) string {
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

func DescribeOrphans(channels []discordsvc.Channel) string {
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

func SuggestChannelProfiles(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, sinceHours int) string {
	profileByChannel := make(map[string]memory.ChannelProfile, len(profiles))
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}
	activityByChannel := make(map[string]memory.ChannelActivity, len(activity))
	for _, item := range activity {
		activityByChannel[item.ChannelID] = item
	}

	lines := []string{fmt.Sprintf("profile suggestions over last %dh:", sinceHours)}
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

func truncateText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
