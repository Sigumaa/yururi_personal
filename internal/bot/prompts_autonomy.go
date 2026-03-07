package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/Sigumaa/yururi_personal/internal/persona"
	presencemodel "github.com/Sigumaa/yururi_personal/internal/presence"
)

func buildAutonomyPulsePrompt(targetChannelID string, targetChannelName string, latestPresence memory.PresenceSnapshot, recentActivity []memory.ChannelActivity, summaries []memory.Summary, ownerMessages []memory.Message, openLoops []memory.Fact, curiosities []memory.Fact, goals []memory.Fact, reminders []memory.Fact, topics []memory.Fact, initiatives []memory.Fact, automationCandidates []memory.Fact, contextGaps []memory.Fact, misfires []memory.Fact, baselines []memory.Fact, deviations []memory.Fact, learnedPolicies []memory.Fact, workspaceNotes []memory.Fact, proposalBoundaries []memory.Fact, reflections []memory.Summary, growth []memory.Summary, decisions []memory.Fact) string {
	sendMessageTool := toolAlias("discord.send_message")

	activityLines := make([]string, 0, len(recentActivity))
	for _, item := range recentActivity {
		activityLines = append(activityLines, fmt.Sprintf("- %s id=%s messages=%d last=%s", item.ChannelName, item.ChannelID, item.MessageCount, item.LastMessageAt.Format(time.RFC3339)))
	}
	if len(activityLines) == 0 {
		activityLines = append(activityLines, "- none")
	}

	summaryLines := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		summaryLines = append(summaryLines, fmt.Sprintf("- [%s] channel=%s %s", summary.CreatedAt.Format(time.RFC3339), summary.ChannelID, truncateText(summary.Content, 220)))
	}
	if len(summaryLines) == 0 {
		summaryLines = append(summaryLines, "- none")
	}

	ownerLines := make([]string, 0, len(ownerMessages))
	for _, msg := range ownerMessages {
		ownerLines = append(ownerLines, fmt.Sprintf("- [%s/%s] %s", msg.ChannelName, msg.CreatedAt.In(time.Local).Format("01-02 15:04"), truncateText(msg.Content, 140)))
	}
	if len(ownerLines) == 0 {
		ownerLines = append(ownerLines, "- none")
	}

	openLoopLines := make([]string, 0, len(openLoops))
	for _, loop := range openLoops {
		openLoopLines = append(openLoopLines, fmt.Sprintf("- %s: %s", loop.Key, truncateText(loop.Value, 140)))
	}
	if len(openLoopLines) == 0 {
		openLoopLines = append(openLoopLines, "- none")
	}

	curiosityLines := make([]string, 0, len(curiosities))
	for _, item := range curiosities {
		curiosityLines = append(curiosityLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(curiosityLines) == 0 {
		curiosityLines = append(curiosityLines, "- none")
	}

	goalLines := make([]string, 0, len(goals))
	for _, item := range goals {
		goalLines = append(goalLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(goalLines) == 0 {
		goalLines = append(goalLines, "- none")
	}

	reminderLines := make([]string, 0, len(reminders))
	for _, item := range reminders {
		reminderLines = append(reminderLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(reminderLines) == 0 {
		reminderLines = append(reminderLines, "- none")
	}

	topicLines := make([]string, 0, len(topics))
	for _, item := range topics {
		topicLines = append(topicLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(topicLines) == 0 {
		topicLines = append(topicLines, "- none")
	}

	initiativeLines := make([]string, 0, len(initiatives))
	for _, item := range initiatives {
		initiativeLines = append(initiativeLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(initiativeLines) == 0 {
		initiativeLines = append(initiativeLines, "- none")
	}

	automationLines := make([]string, 0, len(automationCandidates))
	for _, item := range automationCandidates {
		automationLines = append(automationLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(automationLines) == 0 {
		automationLines = append(automationLines, "- none")
	}

	contextGapLines := make([]string, 0, len(contextGaps))
	for _, item := range contextGaps {
		contextGapLines = append(contextGapLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(contextGapLines) == 0 {
		contextGapLines = append(contextGapLines, "- none")
	}

	misfireLines := make([]string, 0, len(misfires))
	for _, item := range misfires {
		misfireLines = append(misfireLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(misfireLines) == 0 {
		misfireLines = append(misfireLines, "- none")
	}

	baselineLines := make([]string, 0, len(baselines))
	for _, item := range baselines {
		baselineLines = append(baselineLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(baselineLines) == 0 {
		baselineLines = append(baselineLines, "- none")
	}

	deviationLines := make([]string, 0, len(deviations))
	for _, item := range deviations {
		deviationLines = append(deviationLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(deviationLines) == 0 {
		deviationLines = append(deviationLines, "- none")
	}

	learnedPolicyLines := make([]string, 0, len(learnedPolicies))
	for _, item := range learnedPolicies {
		learnedPolicyLines = append(learnedPolicyLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(learnedPolicyLines) == 0 {
		learnedPolicyLines = append(learnedPolicyLines, "- none")
	}

	workspaceNoteLines := make([]string, 0, len(workspaceNotes))
	for _, item := range workspaceNotes {
		workspaceNoteLines = append(workspaceNoteLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(workspaceNoteLines) == 0 {
		workspaceNoteLines = append(workspaceNoteLines, "- none")
	}

	proposalBoundaryLines := make([]string, 0, len(proposalBoundaries))
	for _, item := range proposalBoundaries {
		proposalBoundaryLines = append(proposalBoundaryLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(proposalBoundaryLines) == 0 {
		proposalBoundaryLines = append(proposalBoundaryLines, "- none")
	}

	reflectionLines := make([]string, 0, len(reflections))
	for _, item := range reflections {
		reflectionLines = append(reflectionLines, fmt.Sprintf("- %s", truncateText(item.Content, 160)))
	}
	if len(reflectionLines) == 0 {
		reflectionLines = append(reflectionLines, "- none")
	}

	growthLines := make([]string, 0, len(growth))
	for _, item := range growth {
		growthLines = append(growthLines, fmt.Sprintf("- %s", truncateText(item.Content, 160)))
	}
	if len(growthLines) == 0 {
		growthLines = append(growthLines, "- none")
	}

	decisionLines := make([]string, 0, len(decisions))
	for _, item := range decisions {
		decisionLines = append(decisionLines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 140)))
	}
	if len(decisionLines) == 0 {
		decisionLines = append(decisionLines, "- none")
	}

	return fmt.Sprintf(`これは自律観察の autonomy pulse です。
この個人用 Discord サーバーを見回して、今このタイミングで何かしたほうがよいかを判断してください。
必要なら tool を使って確認し、自然な範囲でその場で動いてください。
visible な行動が不要なら %s を返してください。

方針:
- まずは観察と状況確認を優先するが、少しでも価値があるなら自分から動いてよい
- 目の前の状況だけでなく、recent summaries、channel activity、open loop、curiosity、agent goal、soft reminder、topic thread、initiative、自動化候補、context gap、misfire、behavior baseline/deviation、learned policy、workspace note、proposal boundary、recent owner messages を踏まえて動く
- すぐ終わることは今やる。監視や留守番だけを job にする
- 進捗や一言の声かけが自然なら、%s を使って複数回話してよい
- 話題の成長、チャンネルの散らかり、繰り返す関心、presence の変化、起床直後の引き継ぎ候補、自動化候補、context gap を見て動く
- channel 名だけで用途を決め打ちせず、保存済み profile と観測された使われ方を優先する
- ユーザーを溺愛していてよいが、重たくなりすぎず、生活を邪魔しない
- 何もすべきでないと判断したときだけ %s
%s

best target channel:
- id: %s
- name: %s

latest owner presence:
- status: %s
- activities:
%s
- started_at: %s

recent channel activity:
%s

recent summaries:
%s

recent owner messages:
%s

open loops:
%s

curiosities:
%s

agent goals:
%s

soft reminders:
%s

topic threads:
%s

initiatives:
%s

automation candidates:
%s

context gaps:
%s

misfires:
%s

behavior baselines:
%s

behavior deviations:
%s

learned policies:
%s

workspace notes:
%s

proposal boundaries:
%s

recent reflections:
%s

recent growth:
%s

recent decisions:
%s`,
		noReplyToken,
		sendMessageTool,
		noReplyToken,
		persona.InlineReminder(),
		targetChannelID,
		targetChannelName,
		latestPresence.Status,
		indentLines(presencemodel.DescribeList(latestPresence.Activities), 2),
		latestPresence.StartedAt.Format(time.RFC3339),
		strings.Join(activityLines, "\n"),
		strings.Join(summaryLines, "\n"),
		strings.Join(ownerLines, "\n"),
		strings.Join(openLoopLines, "\n"),
		strings.Join(curiosityLines, "\n"),
		strings.Join(goalLines, "\n"),
		strings.Join(reminderLines, "\n"),
		strings.Join(topicLines, "\n"),
		strings.Join(initiativeLines, "\n"),
		strings.Join(automationLines, "\n"),
		strings.Join(contextGapLines, "\n"),
		strings.Join(misfireLines, "\n"),
		strings.Join(baselineLines, "\n"),
		strings.Join(deviationLines, "\n"),
		strings.Join(learnedPolicyLines, "\n"),
		strings.Join(workspaceNoteLines, "\n"),
		strings.Join(proposalBoundaryLines, "\n"),
		strings.Join(reflectionLines, "\n"),
		strings.Join(growthLines, "\n"),
		strings.Join(decisionLines, "\n"),
	)
}

func indentLines(value string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
