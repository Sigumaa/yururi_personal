package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (a *App) registerMemoryAutonomyTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.list_routines",
		Description: "生活リズムや反復している行動のメモを一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		routines, err := a.store.ListFacts(ctx, "routine", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(routines) == 0 {
			return textTool("no routines"), nil
		}
		lines := make([]string, 0, len(routines))
		for _, routine := range routines {
			lines = append(lines, fmt.Sprintf("- %s: %s", routine.Key, routine.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_routine",
		Description: "生活リズムや反復行動のメモを routine として保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "routine の一意キー"),
			fieldSchema("value", "string", "routine の説明"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Value) == "" {
			return codex.ToolResponse{}, errors.New("key and value are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            "routine",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_pending_promises",
		Description: "まだ完了していない約束や引き受けたことを一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		promises, err := a.store.ListFacts(ctx, "pending_promise", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(promises) == 0 {
			return textTool("no pending promises"), nil
		}
		lines := make([]string, 0, len(promises))
		for _, promise := range promises {
			lines = append(lines, fmt.Sprintf("- %s: %s", promise.Key, promise.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_pending_promise",
		Description: "引き受けた依頼や未完了の約束を pending promise として保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "promise の一意キー"),
			fieldSchema("value", "string", "promise の説明"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Value) == "" {
			return codex.ToolResponse{}, errors.New("key and value are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            "pending_promise",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.close_pending_promise",
		Description: "完了した promise を閉じて、必要なら decision に解決内容を残す",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "閉じる promise のキー"),
			fieldSchema("resolution", "string", "完了内容。省略可"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Key             string `json:"key"`
			Resolution      string `json:"resolution"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" {
			return codex.ToolResponse{}, errors.New("key is required")
		}
		if err := a.store.DeleteFact(ctx, "pending_promise", input.Key); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Resolution) != "" {
			if err := a.store.UpsertFact(ctx, memory.Fact{
				Kind:            "decision",
				Key:             "promise/" + input.Key,
				Value:           input.Resolution,
				SourceMessageID: input.SourceMessageID,
			}); err != nil {
				return codex.ToolResponse{}, err
			}
		}
		return textTool("closed"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_automation_candidates",
		Description: "自動化したい反復作業の候補を一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		candidates, err := a.store.ListFacts(ctx, "automation_candidate", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(candidates) == 0 {
			return textTool("no automation candidates"), nil
		}
		lines := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			lines = append(lines, fmt.Sprintf("- %s: %s", candidate.Key, candidate.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_automation_candidate",
		Description: "反復している依頼や自動化候補を記録する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "候補の一意キー"),
			fieldSchema("value", "string", "候補の説明"),
			fieldSchema("source_message_id", "string", "元メッセージ ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Value) == "" {
			return codex.ToolResponse{}, errors.New("key and value are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            "automation_candidate",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

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
		items, err := a.store.ListFacts(ctx, kind, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(items) == 0 {
			return textTool(emptyText), nil
		}
		lines := make([]string, 0, len(items))
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
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
		var input struct {
			Key             string `json:"key"`
			Value           string `json:"value"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Value) == "" {
			return codex.ToolResponse{}, errors.New("key and value are required")
		}
		if err := a.store.UpsertFact(ctx, memory.Fact{
			Kind:            kind,
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
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
		var input struct {
			Key             string `json:"key"`
			Resolution      string `json:"resolution"`
			SourceMessageID string `json:"source_message_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Key) == "" {
			return codex.ToolResponse{}, errors.New("key is required")
		}
		if err := a.store.DeleteFact(ctx, kind, input.Key); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Resolution) != "" {
			if err := a.store.UpsertFact(ctx, memory.Fact{
				Kind:            resolutionKind,
				Key:             resolutionPrefix + input.Key,
				Value:           input.Resolution,
				SourceMessageID: input.SourceMessageID,
			}); err != nil {
				return codex.ToolResponse{}, err
			}
		}
		return textTool("closed"), nil
	})
}
