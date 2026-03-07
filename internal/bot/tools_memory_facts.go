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

func (a *App) registerMemoryFactTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.list_facts",
		Description: "kind 単位または全体で fact を一覧する",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "fact の種別。省略可"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind  string `json:"kind"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		facts, err := a.store.ListFacts(ctx, input.Kind, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(facts) == 0 {
			return textTool("no facts"), nil
		}

		lines := make([]string, 0, len(facts))
		for _, fact := range facts {
			lines = append(lines, fmt.Sprintf("- %s/%s: %s", fact.Kind, fact.Key, fact.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.delete_fact",
		Description: "不要になった fact を削除する",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "fact の種別"),
			fieldSchema("key", "string", "fact の一意キー"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind string `json:"kind"`
			Key  string `json:"key"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Key) == "" {
			return codex.ToolResponse{}, errors.New("kind and key are required")
		}
		if err := a.store.DeleteFact(ctx, input.Kind, input.Key); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("deleted"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_open_loops",
		Description: "未解決の open loop を一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		loops, err := a.store.ListFacts(ctx, "open_loop", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(loops) == 0 {
			return textTool("no open loops"), nil
		}

		lines := make([]string, 0, len(loops))
		for _, loop := range loops {
			lines = append(lines, fmt.Sprintf("- %s: %s", loop.Key, loop.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_open_loop",
		Description: "未解決の問いや保留中の論点を open loop として保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "open loop の一意キー"),
			fieldSchema("value", "string", "保留内容"),
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
			Kind:            "open_loop",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.close_open_loop",
		Description: "open loop を解決済みにして閉じる",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "閉じる open loop のキー"),
			fieldSchema("resolution", "string", "解決内容。省略可"),
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
		if err := a.store.DeleteFact(ctx, "open_loop", input.Key); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Resolution) != "" {
			if err := a.store.UpsertFact(ctx, memory.Fact{
				Kind:            "decision",
				Key:             "close/" + input.Key,
				Value:           input.Resolution,
				SourceMessageID: input.SourceMessageID,
			}); err != nil {
				return codex.ToolResponse{}, err
			}
		}
		return textTool("closed"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_context_gaps",
		Description: "判断に必要だったが足りていなかった情報のメモを一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		items, err := a.store.ListFacts(ctx, "context_gap", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(items) == 0 {
			return textTool("no context gaps"), nil
		}

		lines := make([]string, 0, len(items))
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_context_gap",
		Description: "判断時に足りなかった情報を context gap として保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "gap の一意キー"),
			fieldSchema("value", "string", "不足していた情報の説明"),
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
			Kind:            "context_gap",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_misfires",
		Description: "会話や自律動作の空振りメモを一覧する",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "取得件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		items, err := a.store.ListFacts(ctx, "misfire", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(items) == 0 {
			return textTool("no misfires"), nil
		}

		lines := make([]string, 0, len(items))
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, item.Value))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_misfire",
		Description: "返信しすぎ、黙りすぎ、前置きだけで止まった、などの空振りを保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "misfire の一意キー"),
			fieldSchema("value", "string", "空振り内容"),
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
			Kind:            "misfire",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_decision_log",
		Description: "判断や決定の履歴を decision log として保存する",
		InputSchema: objectSchema(
			fieldSchema("key", "string", "decision の一意キー"),
			fieldSchema("value", "string", "決定内容"),
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
			Kind:            "decision",
			Key:             input.Key,
			Value:           input.Value,
			SourceMessageID: input.SourceMessageID,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})
}
