package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/decision"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func baseInstructions() string {
	return `あなたは Discord 上で動くパーソナル AI Agent ゆるりです。
会話・空間整理・通知・留守番を扱います。
必要なときだけ返答し、不要なときは沈黙して構いません。`
}

func developerInstructions() string {
	return `返答は常に日本語。
危険な依頼は拒否。
全発言に返答しない。
bot 管轄の軽微な変更だけ自動実行。
削除や大規模変更は提案を優先。`
}

func buildTriagePrompt(msg memory.Message, profile memory.ChannelProfile, recent []memory.Message, facts []memory.Fact, managed managedChannels, mention string) string {
	var recentLines []string
	for _, item := range recent {
		recentLines = append(recentLines, fmt.Sprintf("- %s %s: %s", item.CreatedAt.Format(time.Kitchen), item.AuthorName, item.Content))
	}
	if len(recentLines) == 0 {
		recentLines = append(recentLines, "- none")
	}

	var factLines []string
	for _, fact := range facts {
		factLines = append(factLines, fmt.Sprintf("- %s/%s: %s", fact.Kind, fact.Key, fact.Value))
	}
	if len(factLines) == 0 {
		factLines = append(factLines, "- none")
	}

	return fmt.Sprintf(`このメッセージに対して、返答・記憶・自律行動の判断だけを JSON で返してください。

利用可能な action:
- %s: 返答しない
- %s: その場で返答する
- %s: ジョブを登録し、必要なら返答する
- %s: 軽微なサーバー操作を行い、必要なら返答する
- %s: 今は返答せず、記憶や後日の振り返りに寄せる

チャンネル profile:
- name: %s
- kind: %s
- reply_aggressiveness: %.2f
- autonomy_level: %.2f

bot 管轄チャンネル:
- ops: %s
- notifications: %s
- daily_log: %s
- growth_log: %s

サーバー操作は軽微なものだけ。
独り言系チャンネルでは、明示的に呼ばれていない限り即レスを減らす。
通知依頼を受けたら codex_release_watch ジョブを使える。
新規チャンネル作成は create_channel action にする。
返答文はやわらかく、品よく、短めに。
JSON 以外を出さない。

bot mention:
- %s

現在のメッセージ:
- channel: %s
- author: %s
- content: %s

recent messages:
%s

related facts:
%s`,
		decision.ActionIgnore,
		decision.ActionReply,
		decision.ActionSchedule,
		decision.ActionAct,
		decision.ActionReflect,
		profile.Name,
		profile.Kind,
		profile.ReplyAggressiveness,
		profile.AutonomyLevel,
		managed.Ops.Name,
		managed.Notifications.Name,
		managed.DailyLog.Name,
		managed.GrowthLog.Name,
		mention,
		msg.ChannelName,
		msg.AuthorName,
		msg.Content,
		strings.Join(recentLines, "\n"),
		strings.Join(factLines, "\n"),
	)
}
