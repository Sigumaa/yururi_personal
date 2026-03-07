package bot

import (
	"context"
	"encoding/json"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func (a *App) registerMemoryFactLifecycleTools(registry *codex.ToolRegistry) {
	a.registerFactListTool(registry, "memory.list_curiosities", "自分で調べてみる価値がありそうな疑問メモを一覧する", "curiosity", "no curiosities")
	a.registerFactWriteTool(registry, "memory.write_curiosity", "自分で調べてみたい疑問や引っかかりを curiosity として保存する", "curiosity")
	a.registerFactCloseTool(registry, "memory.resolve_curiosity", "curiosity を解決済みにして閉じ、必要なら decision に残す", "curiosity", "decision", "curiosity/")

	a.registerFactListTool(registry, "memory.list_agent_goals", "自分で追っている目標ややりたいことを一覧する", "agent_goal", "no agent goals")
	a.registerFactWriteTool(registry, "memory.write_agent_goal", "自分で追いたい目標や継続方針を agent goal として保存する", "agent_goal")
	a.registerFactCloseTool(registry, "memory.close_agent_goal", "agent goal を完了や保留にして閉じ、必要なら decision に残す", "agent_goal", "decision", "goal/")

	a.registerFactListTool(registry, "memory.list_soft_reminders", "曖昧な未来表現や、いつか拾いたい予定メモを一覧する", "soft_reminder", "no soft reminders")
	a.registerFactWriteTool(registry, "memory.write_soft_reminder", "あとで、来月、そのうち、のような曖昧な未来メモを soft reminder として保存する", "soft_reminder")
	a.registerFactCloseTool(registry, "memory.complete_soft_reminder", "soft reminder を完了として閉じ、必要なら decision に残す", "soft_reminder", "decision", "reminder/")

	a.registerFactListTool(registry, "memory.list_topic_threads", "最近まとまりつつある話題や思考の束を一覧する", "topic_thread", "no topic threads")
	a.registerFactWriteTool(registry, "memory.write_topic_thread", "散らばったメモや会話を topic thread として束ねて保存する", "topic_thread")

	a.registerFactListTool(registry, "memory.list_initiatives", "自分からやる価値がありそうな整理や提案のメモを一覧する", "initiative", "no initiatives")
	a.registerFactWriteTool(registry, "memory.write_initiative", "自分からやりたいことや提案候補を initiative として保存する", "initiative")

	a.registerFactListTool(registry, "memory.list_behavior_baselines", "いつもの行動や空気感の基準メモを一覧する", "behavior_baseline", "no behavior baselines")
	a.registerFactWriteTool(registry, "memory.write_behavior_baseline", "いつもの行動や空気感の基準を behavior baseline として保存する", "behavior_baseline")

	a.registerFactListTool(registry, "memory.list_behavior_deviations", "いつもと違う動きや空気感の観測メモを一覧する", "behavior_deviation", "no behavior deviations")
	a.registerFactWriteTool(registry, "memory.write_behavior_deviation", "いつもと違う動きや空気感の観測を behavior deviation として保存する", "behavior_deviation")

	a.registerFactListTool(registry, "memory.list_learned_policies", "経験からにじんだ振る舞い方針のメモを一覧する", "learned_policy", "no learned policies")
	a.registerFactWriteTool(registry, "memory.write_learned_policy", "経験から学んだ軽い振る舞い方針を learned policy として保存する", "learned_policy")
	a.registerFactCloseTool(registry, "memory.retire_learned_policy", "古くなった learned policy を退役させ、必要なら decision に残す", "learned_policy", "decision", "policy/")

	a.registerFactListTool(registry, "memory.list_workspace_notes", "下書きや途中メモとして残している workspace note を一覧する", "workspace_note", "no workspace notes")
	a.registerFactWriteTool(registry, "memory.write_workspace_note", "自分用の下書きや途中メモを workspace note として保存する", "workspace_note")

	a.registerFactListTool(registry, "memory.list_proposal_boundaries", "勝手にやる・提案に留める・観測だけにする境界メモを一覧する", "proposal_boundary", "no proposal boundaries")
	a.registerFactWriteTool(registry, "memory.write_proposal_boundary", "自律的にやることと提案に留めることの境界メモを proposal boundary として保存する", "proposal_boundary")
	a.registerFactCloseTool(registry, "memory.retire_proposal_boundary", "古くなった proposal boundary を退役させ、必要なら decision に残す", "proposal_boundary", "decision", "boundary/")
}

func (a *App) registerFactListTool(registry *codex.ToolRegistry, toolName string, description string, kind string, emptyText string) {
	registry.Register(codex.ToolSpec{
		Name:        toolName,
		Description: description,
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		return a.listFactText(ctx, kind, input.Limit, emptyText)
	})
}

func (a *App) registerFactWriteTool(registry *codex.ToolRegistry, toolName string, description string, kind string) {
	registry.Register(codex.ToolSpec{
		Name:        toolName,
		Description: description,
		InputSchema: objectSchema(
			fieldSchema("key", "string", "一意キー"),
			fieldSchema("value", "string", "保存内容"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		return a.writeFact(ctx, raw, kind)
	})
}

func (a *App) registerFactCloseTool(registry *codex.ToolRegistry, toolName string, description string, kind string, resolutionKind string, resolutionPrefix string) {
	registry.Register(codex.ToolSpec{
		Name:        toolName,
		Description: description,
		InputSchema: objectSchema(
			fieldSchema("key", "string", "閉じる項目のキー"),
			fieldSchema("resolution", "string", "完了や保留の内容。省略可"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		return a.closeFact(ctx, raw, kind, resolutionKind, resolutionPrefix)
	})
}
