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

func (a *App) registerMemorySpecializedTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.list_routines",
		Description: "生活リズムや反復している行動のメモを一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		return a.listFactText(ctx, "routine", input.Limit, "no routines")
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
		return a.writeFact(ctx, raw, "routine")
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
		return a.listFactText(ctx, "pending_promise", input.Limit, "no pending promises")
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
		return a.writeFact(ctx, raw, "pending_promise")
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
		return a.closeFact(ctx, raw, "pending_promise", "decision", "promise/")
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
		return a.listFactText(ctx, "automation_candidate", input.Limit, "no automation candidates")
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
		return a.writeFact(ctx, raw, "automation_candidate")
	})
}

func (a *App) listFactText(ctx context.Context, kind string, limit int, emptyText string) (codex.ToolResponse, error) {
	items, err := a.store.ListFacts(ctx, kind, limit)
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
}

func (a *App) writeFact(ctx context.Context, raw json.RawMessage, kind string) (codex.ToolResponse, error) {
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
}

func (a *App) closeFact(ctx context.Context, raw json.RawMessage, kind string, resolutionKind string, resolutionPrefix string) (codex.ToolResponse, error) {
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
}
