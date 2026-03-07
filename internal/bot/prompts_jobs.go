package bot

import (
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

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
	describeServerTool := toolAlias("discord.describe_server")
	listJobsTool := toolAlias("jobs.list")
	listProfilesTool := toolAlias("memory.list_channel_profiles")
	readMessagesTool := toolAlias("discord.read_recent_messages")

	return fmt.Sprintf(`これはユーザーへ見せない内部用の background task 実行です。
ここで必要なのは説明や着手宣言ではなく、実際の確認と実行です。

ルール:
- まず必要な tool を使って状況を確認し、事実ベースで進める
- これからやる、あとで返す、確認する、といった未完了の約束文は禁止
- tool を使わずに、できない・接続できない・確認できないと決めつけない
- 接続不可や失敗を述べるときは、実際に失敗した tool 名とエラー内容をそのまま含める
- サーバー俯瞰や機能確認なら、必要に応じて %s, %s, %s, %s を使う
- 返答は、作業後の完成した結果だけを書く

job:
- id: %s
- title: %s
- channel_id: %s

task:
%s`, describeServerTool, listJobsTool, listProfilesTool, readMessagesTool, job.ID, job.Title, job.ChannelID, strings.TrimSpace(prompt))
}
