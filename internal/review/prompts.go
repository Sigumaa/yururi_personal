package review

import (
	"fmt"
	"strings"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/Sigumaa/yururi_personal/internal/space"
)

func OpenLoopPrompt(noReply string, loops []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`未解決の open loop を軽く見直してください。
今すぐ返す価値があるもの、しばらく寝かせてよいもの、自分で調べてよさそうなものを 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

open loops:
%s

recent owner messages:
%s`, noReply, joinLines(formatFactLines(loops, 0)), joinLines(formatMessageLines(recentOwnerMessages, 180)))
}

func CuriosityPrompt(noReply string, curiosities []memory.Fact, openLoops []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`curiosity を見直してください。
自分で調べる価値が高そうなもの、まだ寝かせるもの、open loop に戻したほうがよさそうなものを 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

curiosities:
%s

open loops:
%s

recent owner messages:
%s`, noReply, joinLines(formatFactLines(curiosities, 0)), joinLines(formatFactLines(openLoops, 180)), joinLines(formatMessageLines(recentOwnerMessages, 180)))
}

func InitiativePrompt(noReply string, initiatives []memory.Fact, candidates []memory.Fact, openLoops []memory.Fact, contextGaps []memory.Fact) string {
	return fmt.Sprintf(`initiative と automation candidate を見直してください。
いま動く価値があるもの、まだ材料不足なもの、自動化候補として育てるとよさそうなものを 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

initiatives:
%s

automation candidates:
%s

open loops:
%s

context gaps:
%s`, noReply, joinLines(formatFactLines(initiatives, 0)), joinLines(formatFactLines(candidates, 180)), joinLines(formatFactLines(openLoops, 180)), joinLines(formatFactLines(contextGaps, 180)))
}

func SoftReminderPrompt(noReply string, reminders []memory.Fact, routines []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`soft reminder を見直してください。
そろそろ触れてよさそうなもの、まだ寝かせるもの、routine と結びつきが強そうなものを 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

soft reminders:
%s

routines:
%s

recent owner messages:
%s`, noReply, joinLines(formatFactLines(reminders, 0)), joinLines(formatFactLines(routines, 180)), joinLines(formatMessageLines(recentOwnerMessages, 180)))
}

func TopicSynthesisPrompt(noReply string, topics []memory.Fact, recentOwnerMessages []memory.Message, summaries []memory.Summary) string {
	return fmt.Sprintf(`散らばった話題を topic thread として見直してください。
最近まとまってきた話題、薄くなった話題、今まとめ直すとよさそうな話題を 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

topic threads:
%s

recent owner messages:
%s

recent summaries:
%s`, noReply, joinLines(formatFactLines(topics, 0)), joinLines(formatMessageLines(recentOwnerMessages, 180)), joinLines(formatSummaryLines(summaries, 180)))
}

func BaselinePrompt(noReply string, baselines []memory.Fact, deviations []memory.Fact, routines []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`behavior baseline と deviation を見直してください。
いつも通りとして持っておいてよさそうなもの、最近のズレとして気にしておくとよさそうなものを 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

behavior baselines:
%s

behavior deviations:
%s

routines:
%s

recent owner messages:
%s`, noReply, joinLines(formatFactLines(baselines, 180)), joinLines(formatFactLines(deviations, 180)), joinLines(formatFactLines(routines, 180)), joinLines(formatMessageLines(recentOwnerMessages, 180)))
}

func PolicySynthesisPrompt(noReply string, learnedPolicies []memory.Fact, decisions []memory.Fact, misfires []memory.Fact, reflections []memory.Summary) string {
	return fmt.Sprintf(`経験からにじんだ learned policy を見直してください。
続けるとよさそうな方針、弱めたほうがよさそうな方針、まだ仮置きにしておく方針を 1 から 3 個だけ短くまとめてください。
基底人格や固定ルールを書き換えるのではなく、最近の成功・失敗から学べる軽い方針として扱ってください。
必要性が薄ければ %s だけを返してください。

learned policies:
%s

recent decisions:
%s

recent misfires:
%s

recent reflections:
%s`, noReply, joinLines(formatFactLines(learnedPolicies, 0)), joinLines(formatFactLines(decisions, 180)), joinLines(formatFactLines(misfires, 180)), joinLines(formatSummaryLines(reflections, 180)))
}

func WorkspacePrompt(noReply string, workspaceNotes []memory.Fact, initiatives []memory.Fact, topics []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`自分用の workspace note や途中メモを見直してください。
下書きとして育てる価値があるもの、そろそろ片づけてよいもの、チャンネルや作業場所として形にしたほうがよさそうなものを短くまとめてください。
必要性が薄ければ %s だけを返してください。

workspace notes:
%s

initiatives:
%s

topic threads:
%s

recent owner messages:
%s`, noReply, joinLines(formatFactLines(workspaceNotes, 180)), joinLines(formatFactLines(initiatives, 180)), joinLines(formatFactLines(topics, 180)), joinLines(formatMessageLines(recentOwnerMessages, 180)))
}

func ProposalBoundaryPrompt(noReply string, proposalBoundaries []memory.Fact, initiatives []memory.Fact, decisions []memory.Fact, misfires []memory.Fact, contextGaps []memory.Fact) string {
	return fmt.Sprintf(`自分から勝手にやること、提案に留めること、観測だけしておくことの境界を見直してください。
固定ルールではなく、最近の成功・失敗・迷いから学べる境界メモとして 1 から 3 個だけ短くまとめてください。
必要性が薄ければ %s だけを返してください。

proposal boundaries:
%s

initiatives:
%s

recent decisions:
%s

recent misfires:
%s

context gaps:
%s`, noReply, joinLines(formatFactLines(proposalBoundaries, 180)), joinLines(formatFactLines(initiatives, 180)), joinLines(formatFactLines(decisions, 180)), joinLines(formatFactLines(misfires, 180)), joinLines(formatFactLines(contextGaps, 180)))
}

func DecisionPrompt(noReply string, decisions []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`最近の decision を軽く見直して、いまの会話傾向に対して効きそうなものだけを短くまとめてください。
古い判断を盲信せず、続けるもの、弱めるもの、あとで見直すものを柔らかく示してください。
必要性が薄ければ %s だけを返してください。

recent decisions:
%s

recent owner messages:
%s`, noReply, joinLines(formatFactLines(decisions, 0)), joinLines(formatMessageLines(recentOwnerMessages, 180)))
}

func SelfImprovementPrompt(noReply string, candidates []memory.Fact, reflections []memory.Summary, growth []memory.Summary) string {
	return fmt.Sprintf(`自分の振る舞い改善を 1 回だけ見直してください。
自動化候補、reflection、growth から、次に効きそうな改善の種を 1 から 3 個だけ短くまとめてください。
実装を決め打ちせず、観測、整備、記録の観点で書いてください。
必要性が薄ければ %s だけを返してください。

automation candidates:
%s

reflections:
%s

growth:
%s`, noReply, joinLines(formatFactLines(candidates, 0)), joinLines(formatSummaryLines(reflections, 180)), joinLines(formatSummaryLines(growth, 180)))
}

func ChannelCurationPrompt(noReply string, channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity) string {
	return fmt.Sprintf(`Discord 空間の整理観点を 1 回だけ提案してください。
やることは即実行ではなく、いま気づいた整理候補の提示です。
次の 3 点を短く出してください。
1. 最近育っている場所
2. 静かで整理候補になりそうな場所
3. 次に 1 つだけやるなら何か
必要な提案が特に無ければ %s だけを返してください。

server snapshot:
%s`, noReply, space.DescribeServer(channels, profiles, activity, time.UTC))
}

func ChannelRolePrompt(noReply string, channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity) string {
	return fmt.Sprintf(`Discord 空間の channel role を見直してください。
今ある profile を固定前提にせず、最近の活動とカテゴリ構造から、役割 drift が起きていそうな場所だけを短く示してください。
必要なら profile をどう寄せるとよさそうかを 1 行ずつ添えてください。
必要性が薄ければ %s だけを返してください。

server snapshot:
%s`, noReply, space.DescribeServer(channels, profiles, activity, time.UTC))
}

func formatFactLines(items []memory.Fact, truncateAt int) []string {
	if len(items) == 0 {
		return []string{"- none"}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		value := item.Value
		if truncateAt > 0 {
			value = truncateText(value, truncateAt)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, value))
	}
	return lines
}

func formatMessageLines(items []memory.Message, truncateAt int) []string {
	if len(items) == 0 {
		return []string{"- none"}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- [%s/%s] %s", item.CreatedAt.Format("01-02 15:04"), item.ChannelName, truncateText(item.Content, truncateAt)))
	}
	return lines
}

func formatSummaryLines(items []memory.Summary, truncateAt int) []string {
	if len(items) == 0 {
		return []string{"- none"}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s", truncateText(item.Content, truncateAt)))
	}
	return lines
}

func joinLines(items []string) string {
	return strings.Join(items, "\n")
}

func truncateText(s string, limit int) string {
	if limit <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}
