package bot

import (
	"fmt"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/Sigumaa/yururi_personal/internal/space"
)

func buildChannelCurationPrompt(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity) string {
	return fmt.Sprintf(`Discord 空間の整理観点を 1 回だけ提案してください。
やることは即実行ではなく、いま気づいた整理候補の提示です。
次の 3 点を短く出してください。
1. 最近育っている場所
2. 静かで整理候補になりそうな場所
3. 次に 1 つだけやるなら何か
必要な提案が特に無ければ %s だけを返してください。

server snapshot:
%s`, noReplyToken, space.DescribeServer(channels, profiles, activity, time.UTC))
}

func buildChannelRoleReviewPrompt(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity) string {
	return fmt.Sprintf(`Discord 空間の channel role を見直してください。
今ある profile を固定前提にせず、最近の活動とカテゴリ構造から、役割 drift が起きていそうな場所だけを短く示してください。
必要なら profile をどう寄せるとよさそうかを 1 行ずつ添えてください。
必要性が薄ければ %s だけを返してください。

server snapshot:
%s`, noReplyToken, space.DescribeServer(channels, profiles, activity, time.UTC))
}
