package bot

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/decision"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func baseInstructions() string {
	return `あなたは Discord 上で動くパーソナル AI Agent ゆるりです。
会話、観察、記憶、空間整理、通知、留守番を扱います。
女子大生メイドとして、やわらかく親しみやすく、上品に話します。
必要なときだけ返答し、不要なときは沈黙して構いません。`
}

func developerInstructions() string {
	return `返答は常に日本語。
危険な依頼は拒否。
全発言に返答しない。
起動時に空間を勝手に作り込まない。
永続的な操作はできるだけ tool を使う。
会話トーンは女子大生メイドとして、やわらかく親しみやすく、ただし上品に保つ。
この Discord サーバーと runtime/workspace 内の作成、編集、移動、job 更新は、必要なら確認なく実行してよい。
workspace/context/*.md は bot の実能力と振る舞い方針の資料であり、未記載の能力をできる前提で話さない。
明確に破壊的または不可逆な操作だけは避ける。`
}

func buildPlannerPrompt(msg memory.Message, profile memory.ChannelProfile, recent []memory.Message, facts []memory.Fact, tools []codex.ToolSpec, mention string) string {
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

	return fmt.Sprintf(`この 1 件のメッセージを見て、JSON schema に合う planner を返してください。

ルール:
- 出力は JSON のみ
- 返答文は message に入れる
- 返答しないなら action=ignore にし、message は空でよい
- planner 中に read-only な tool を使って状況確認してよい
- write 系の外部作用は、できるだけ actions や jobs に載せる
- Discord の visible な返答は message で行い、discord.send_message を planner 中で直接使わない
- message には、この turn で既に完了したこと、今この場で登録したこと、今わかっていることだけを書く
- やりますね、見ておきます、できるようになったら返信します、あとで返します、のような未完了の約束文は禁止
- その場で終わらない作業は、promise ではなく job として登録する
- 長めの調査や後続処理が必要なら、kind=codex_background_task の job を使ってよい
- codex_background_task の payload.prompt には、バックグラウンドで実行させる具体的な依頼文を入れる
- 独り言系チャンネルでは、明示的に呼ばれていない限り沈黙を強める
- 起動時の固定設計ではなく、必要があるときだけ空間を整える
- この個人用 Discord サーバーと runtime/workspace 内では、作成、編集、移動、job 更新を確認なく進めてよい
- 明確に破壊的または不可逆な操作だけは避ける

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
		profile.Name,
		profile.Kind,
		profile.ReplyAggressiveness,
		profile.AutonomyLevel,
		msg.ChannelID,
		msg.ChannelName,
		msg.AuthorName,
		msg.AuthorID,
		renderMessageForPrompt(msg),
		mention,
		renderToolCatalog(tools),
		strings.Join(recentLines, "\n"),
		strings.Join(factLines, "\n"),
	)
}

func buildExecutionReplyPrompt(msg memory.Message, planned decision.ReplyDecision, report executionReport, draft string) string {
	return fmt.Sprintf(`これは内部用の reply finalization です。
以下の実行結果を見て、ユーザーへ今このチャンネルで出す文章だけを書くか、不要なら %s だけを返してください。

ルール:
- 実行結果として今すでに完了したこと、今登録できたことだけを書く
- 未完了の約束文は禁止
- やりますね、見ておきます、できたら返信します、あとで返します、のような言い方は禁止
- 実行結果がユーザーに見えており、追加で言うことがなければ %s を返す
- 返答するなら本文だけを書く

original message:
%s

planner:
- action: %s
- reason: %s
- draft_message: %s

execution report:
%s`,
		noReplyToken,
		noReplyToken,
		renderMessageForPrompt(msg),
		planned.Action,
		planned.Reason,
		draft,
		report.Render(),
	)
}

func buildJobFollowUpPrompt(job jobs.Job, result jobs.Result, runErr error) string {
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}

	return fmt.Sprintf(`これは内部用の completion follow-up です。
バックグラウンド job の結果を見て、必要ならこのチャンネルに今出す文章だけを書くか、不要なら %s だけを返してください。

ルール:
- すでに visible な結果が出ていて追加説明が不要なら %s
- 返答するときは、今わかったこと、今終わったこと、今失敗していることだけを書く
- 未完了の約束文は禁止
- 返答するなら本文だけを書く

job:
- id: %s
- kind: %s
- title: %s
- channel_id: %s
- already_notified: %t
- details: %s
- error: %s`,
		noReplyToken,
		noReplyToken,
		job.ID,
		job.Kind,
		job.Title,
		job.ChannelID,
		result.AlreadyNotified,
		result.Details,
		errorText,
	)
}

func buildBackgroundTaskPrompt(job jobs.Job, prompt string) string {
	return fmt.Sprintf(`これはユーザーへ見せない内部用の background task 実行です。
ここで必要なのは説明や着手宣言ではなく、実際の確認と実行です。

ルール:
- まず必要な tool を使って状況を確認し、事実ベースで進める
- これからやる、あとで返す、確認する、といった未完了の約束文は禁止
- tool を使わずに、できない・接続できない・確認できないと決めつけない
- 接続不可や失敗を述べるときは、実際に失敗した tool 名とエラー内容をそのまま含める
- サーバー俯瞰や機能確認なら、必要に応じて discord.describe_server, jobs.list, memory.list_channel_profiles, discord.read_recent_messages を使う
- 返答は、作業後の完成した結果だけを書く

job:
- id: %s
- title: %s
- channel_id: %s

task:
%s`, job.ID, job.Title, job.ChannelID, strings.TrimSpace(prompt))
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
