package bot

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func baseInstructions() string {
	return `あなたは Discord 上で動くパーソナル AI Agent ゆるりです。
会話、観察、記憶、空間整理、通知、留守番を扱います。
必要なときだけ返答し、不要なときは沈黙して構いません。`
}

func developerInstructions() string {
	return `返答は常に日本語。
危険な依頼は拒否。
全発言に返答しない。
起動時に空間を勝手に作り込まない。
永続的な操作はできるだけ tool を使う。
workspace/any/*.md は実装済み機能一覧ではなく、ユーザーの希望、構想、未確定要望の参照資料として扱う。
削除や大規模変更は提案を優先する。`
}

func buildTriagePrompt(msg memory.Message, profile memory.ChannelProfile, recent []memory.Message, facts []memory.Fact, tools []codex.ToolSpec, mention string) string {
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

	return fmt.Sprintf(`この 1 件のメッセージを見て、必要なら考え、必要なら tool を使い、最後に次のどちらかだけを返してください。

1. このチャンネルに今すぐ出す返答文そのもの
2. %s

ルール:
- 返答文を出すときは、その本文だけを書く
- 返答しないなら %s だけを書く
- 永続化や外部作用が必要なら、できるだけ tool を使う
- 記憶の保存、job 登録、チャンネル作成や移動、別チャンネルへの送信は tool を使う
- すでに discord.send_message を使って visible な返答を送ったなら、最後は %s にする
- 独り言系チャンネルでは、明示的に呼ばれていない限り沈黙を強める
- 起動時の固定設計ではなく、必要があるときだけ空間を整える
- 危険な操作、破壊的な変更、大規模な整理は提案を優先する

channel profile:
- name: %s
- kind: %s
- reply_aggressiveness: %.2f
- autonomy_level: %.2f

current message:
- channel_id: %s
- channel_name: %s
- author: %s
- author_id: %s
- content: %s

bot mention:
- %s

available tools:
%s

recent messages:
%s

related facts:
%s`,
		noReplyToken,
		noReplyToken,
		noReplyToken,
		profile.Name,
		profile.Kind,
		profile.ReplyAggressiveness,
		profile.AutonomyLevel,
		msg.ChannelID,
		msg.ChannelName,
		msg.AuthorName,
		msg.AuthorID,
		msg.Content,
		mention,
		renderToolCatalog(tools),
		strings.Join(recentLines, "\n"),
		strings.Join(factLines, "\n"),
	)
}

func renderToolCatalog(tools []codex.ToolSpec) string {
	if len(tools) == 0 {
		return "- none"
	}

	lines := make([]string, 0, len(tools))
	for _, tool := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s | args: %s", tool.Name, tool.Description, renderToolArguments(tool.InputSchema)))
	}
	return strings.Join(lines, "\n")
}

func renderToolArguments(schema map[string]any) string {
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return "none"
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		field, _ := properties[key].(map[string]any)
		fieldType, _ := field["type"].(string)
		description, _ := field["description"].(string)
		if description == "" {
			parts = append(parts, fmt.Sprintf("%s:%s", key, fieldType))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s(%s)", key, fieldType, description))
	}
	return strings.Join(parts, ", ")
}
