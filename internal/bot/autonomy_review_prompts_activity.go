package bot

import (
	"fmt"

	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func buildOpenLoopReviewPrompt(loops []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`未解決の open loop を見直して、いま声をかける価値があるものだけを日本語で短くまとめてください。
単なる一覧ではなく、いま気にしておいたほうがよいこと、優先度、軽い次の一歩をやわらかく添えてください。
必要性が薄ければ %s だけを返してください。

open loops:
%s

recent owner messages:
%s`, noReplyToken, joinPromptLines(formatFactLines(loops, 0)), joinPromptLines(formatMessageLines(recentOwnerMessages, 180)))
}

func buildCuriosityReviewPrompt(curiosities []memory.Fact, openLoops []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`気になっている疑問を見直して、自分で調べておいたら役に立ちそうなものだけを 1 から 3 個、日本語で短くまとめてください。
ここでは即実行ではなく、調べる価値のある問い、軽い調査の向き先、寝かせてもよい問いを見分けてください。
必要性が薄ければ %s だけを返してください。

curiosities:
%s

open loops:
%s

recent owner messages:
%s`, noReplyToken, joinPromptLines(formatFactLines(curiosities, 0)), joinPromptLines(formatFactLines(openLoops, 0)), joinPromptLines(formatMessageLines(recentOwnerMessages, 180)))
}

func buildInitiativeReviewPrompt(initiatives []memory.Fact, candidates []memory.Fact, openLoops []memory.Fact, contextGaps []memory.Fact) string {
	return fmt.Sprintf(`いまの流れで自分からやる価値があることを見直してください。
分類は、1. 勝手に整えてよい軽い下ごしらえ 2. 提案だけに留めるべきこと 3. まだ様子見 の 3 つです。
必要性が薄ければ %s だけを返してください。

initiatives:
%s

automation candidates:
%s

open loops:
%s

context gaps:
%s`, noReplyToken, joinPromptLines(formatFactLines(initiatives, 0)), joinPromptLines(formatFactLines(candidates, 0)), joinPromptLines(formatFactLines(openLoops, 0)), joinPromptLines(formatFactLines(contextGaps, 0)))
}

func buildSoftReminderReviewPrompt(reminders []memory.Fact, routines []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`曖昧な未来メモを見直して、そろそろ声をかけても自然そうなものだけを短くまとめてください。
来月、そのうち、あとで、のような曖昧さをそのまま扱い、今はまだ早そうなものは寝かせてください。
必要性が薄ければ %s だけを返してください。

soft reminders:
%s

routines:
%s

recent owner messages:
%s`, noReplyToken, joinPromptLines(formatFactLines(reminders, 0)), joinPromptLines(formatFactLines(routines, 0)), joinPromptLines(formatMessageLines(recentOwnerMessages, 180)))
}

func buildTopicSynthesisReviewPrompt(topics []memory.Fact, recentOwnerMessages []memory.Message, summaries []memory.Summary) string {
	return fmt.Sprintf(`散らばった話題を束ね直して、今まとまり始めているテーマがあるかを短くまとめてください。
単なる日報ではなく、別チャンネルや別の日に散らばった断片が、どの話題へ寄っているかを見る観点で書いてください。
必要性が薄ければ %s だけを返してください。

topic threads:
%s

recent owner messages:
%s

recent weekly summaries:
%s`, noReplyToken, joinPromptLines(formatFactLines(topics, 0)), joinPromptLines(formatMessageLines(recentOwnerMessages, 160)), joinPromptLines(formatSummaryLines(summaries, 180)))
}

func buildBaselineReviewPrompt(baselines []memory.Fact, deviations []memory.Fact, routines []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`いつもと違う気配を見直して、口に出すほどではない観測と、軽く寄り添ったほうがよさそうな違いを分けて短くまとめてください。
過剰に踏み込みすぎず、見守り寄りの観測として扱ってください。
必要性が薄ければ %s だけを返してください。

behavior baselines:
%s

behavior deviations:
%s

routines:
%s

recent owner messages:
%s`, noReplyToken, joinPromptLines(formatFactLines(baselines, 0)), joinPromptLines(formatFactLines(deviations, 0)), joinPromptLines(formatFactLines(routines, 0)), joinPromptLines(formatMessageLines(recentOwnerMessages, 160)))
}
