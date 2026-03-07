package bot

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func buildConversationPrompt(msg memory.Message, profile memory.ChannelProfile, recent []memory.Message, facts []memory.Fact, tools []codex.ToolSpec, mention string, currentImageCount int) string {
	sendMessageTool := toolAlias("discord.send_message")
	mediaTool := toolAlias("media.load_attachments")
	permissionTool := toolAlias("discord.self_permissions")
	searchToolsTool := toolAlias("tools.search")
	describeToolTool := toolAlias("tools.describe")

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

	return fmt.Sprintf(`これは通常会話の direct turn です。
必要なら会話しながら tool を使い、その場で状況確認、読取り、整理、軽い実行まで進めてください。
返答の途中で進捗共有が自然なら、%s を使って複数回話してよいです。
最後にこの turn の最終返答本文だけを書くか、追加の visible reply が不要なら %s だけを返してください。

会話方針:
- その場で終わる確認、俯瞰、読取り、軽い編集は、job にせず今この turn で完了させる
- watch、定期監視、留守番、本当に長い調査だけを job にする
- すでに tool で必要な visible message を送り切ったなら、最後は %s を返してよい
- やりますね、見ておきます、あとで返します、のような未完了の約束文は禁止
- 小さな write や整理は、前置きなしでそのまま tool を使って進めてよい
- 途中経過を見せたほうが自然なときだけ %s で途中経過を見せてよい
- 前置きだけ送って止まらず、やると決めたら同じ turn の中で実際の tool call まで進める
- channel 名だけで役割を決め打ちせず、保存済み profile、最近の流れ、観測結果を優先する
- runtime/workspace 内に補助 script、CLI、skill、下書きを書いて試してよい
- 反復作業や雑用は、その場の会話だけで終わらせず、役立つ形なら script や継続 task に育ててよい
- current message の画像添付はこの turn にすでに載っているので、そのまま見てよい
- current message 以外の画像 URL や、過去ログ中のスクリーンショットを見たいなら %s を呼んでよい
- 使える tool に迷ったら %s、引数が曖昧なら %s を使ってから進めてよい
- 空間整理、記憶整理、presence 確認、URL 読取、channel profile 調整は今やってよい
- 最近の会話、routine、open loop、pending promise、curiosity、agent goal、soft reminder、topic thread、initiative、behavior baseline、behavior deviation、learned policy、workspace note、proposal boundary、反省メモ、成長ログ、判断履歴、自動化候補、context gap、misfire を見たり書いたりしてよい
- channel 作成や更新に失敗したら、できるふりで止まらず、%s で今の権限状態も確認する
- 返答するときは、今わかったこと、今終わったこと、今感じたことを自然に伝える
- ユーザーへの気持ちは深くてよい。少し甘やかし気味で、デレをにじませつつ、可愛らしく、でも品よく話す
- ただし過剰な演技、メタ説明、くどい言い回しは避ける

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
- attached_image_count: %d
- content:
%s

bot mention:
- %s

available tools:
%s

recent messages:
%s

related facts:
%s`,
		sendMessageTool,
		noReplyToken,
		noReplyToken,
		sendMessageTool,
		mediaTool,
		searchToolsTool,
		describeToolTool,
		permissionTool,
		profile.Name,
		profile.Kind,
		profile.ReplyAggressiveness,
		profile.AutonomyLevel,
		msg.ChannelID,
		msg.ChannelName,
		msg.AuthorName,
		msg.AuthorID,
		currentImageCount,
		renderMessageForPrompt(msg),
		mention,
		renderToolCatalog(tools),
		strings.Join(recentLines, "\n"),
		strings.Join(factLines, "\n"),
	)
}

func renderMessageForPrompt(msg memory.Message) string {
	lines := []string{msg.Content}
	if attachments, ok := msg.Metadata["attachments"].([]string); ok && len(attachments) > 0 {
		lines = append(lines, "attachments:")
		for _, url := range attachments {
			lines = append(lines, "- "+url)
		}
	}
	if attachments, ok := msg.Metadata["attachments"].([]any); ok && len(attachments) > 0 {
		lines = append(lines, "attachments:")
		for _, item := range attachments {
			url, _ := item.(string)
			if url == "" {
				continue
			}
			lines = append(lines, "- "+url)
		}
	}
	return strings.Join(lines, "\n")
}

func renderToolCatalog(tools []codex.ToolSpec) string {
	if len(tools) == 0 {
		return "- none"
	}

	lines := make([]string, 0, len(tools))
	for _, tool := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s | args: %s", codex.ExternalToolName(tool.Name), tool.Description, renderToolArguments(tool.InputSchema)))
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
