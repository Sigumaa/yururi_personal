package bot

import (
	"fmt"

	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func buildPolicySynthesisReviewPrompt(learnedPolicies []memory.Fact, decisions []memory.Fact, misfires []memory.Fact, reflections []memory.Summary) string {
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
%s`, noReplyToken, joinPromptLines(formatFactLines(learnedPolicies, 0)), joinPromptLines(formatFactLines(decisions, 180)), joinPromptLines(formatFactLines(misfires, 180)), joinPromptLines(formatSummaryLines(reflections, 180)))
}

func buildWorkspaceReviewPrompt(workspaceNotes []memory.Fact, initiatives []memory.Fact, topics []memory.Fact, recentOwnerMessages []memory.Message) string {
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
%s`, noReplyToken, joinPromptLines(formatFactLines(workspaceNotes, 180)), joinPromptLines(formatFactLines(initiatives, 180)), joinPromptLines(formatFactLines(topics, 180)), joinPromptLines(formatMessageLines(recentOwnerMessages, 180)))
}

func buildProposalBoundaryReviewPrompt(proposalBoundaries []memory.Fact, initiatives []memory.Fact, decisions []memory.Fact, misfires []memory.Fact, contextGaps []memory.Fact) string {
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
%s`, noReplyToken, joinPromptLines(formatFactLines(proposalBoundaries, 180)), joinPromptLines(formatFactLines(initiatives, 180)), joinPromptLines(formatFactLines(decisions, 180)), joinPromptLines(formatFactLines(misfires, 180)), joinPromptLines(formatFactLines(contextGaps, 180)))
}

func buildDecisionReviewPrompt(decisions []memory.Fact, recentOwnerMessages []memory.Message) string {
	return fmt.Sprintf(`最近の decision を軽く見直して、いまの会話傾向に対して効きそうなものだけを短くまとめてください。
古い判断を盲信せず、続けるもの、弱めるもの、あとで見直すものを柔らかく示してください。
必要性が薄ければ %s だけを返してください。

recent decisions:
%s

recent owner messages:
%s`, noReplyToken, joinPromptLines(formatFactLines(decisions, 0)), joinPromptLines(formatMessageLines(recentOwnerMessages, 180)))
}

func buildSelfImprovementReviewPrompt(candidates []memory.Fact, reflections []memory.Summary, growth []memory.Summary) string {
	return fmt.Sprintf(`自分の振る舞い改善を 1 回だけ見直してください。
自動化候補、reflection、growth から、次に効きそうな改善の種を 1 から 3 個だけ短くまとめてください。
実装を決め打ちせず、観測、整備、記録の観点で書いてください。
必要性が薄ければ %s だけを返してください。

automation candidates:
%s

reflections:
%s

growth:
%s`, noReplyToken, joinPromptLines(formatFactLines(candidates, 0)), joinPromptLines(formatSummaryLines(reflections, 180)), joinPromptLines(formatSummaryLines(growth, 180)))
}
