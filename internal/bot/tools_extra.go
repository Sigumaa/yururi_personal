package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func (a *App) registerExtraTools(registry *codex.ToolRegistry) {
	a.registerToolHelperTools(registry)
	a.registerAutonomyTools(registry)
	a.registerMemoryExtraTools(registry)
	a.registerJobExtraTools(registry)
	a.registerDiscordExtraTools(registry)
	a.registerWebTools(registry)
	a.registerMediaTools(registry)
}

func (a *App) registerToolHelperTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "tools.search",
		Description: "使えそうな tool を名前や説明から探す",
		InputSchema: objectSchema(
			fieldSchema("query", "string", "探したい操作や概念"),
			fieldSchema("limit", "integer", "返す件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if strings.TrimSpace(input.Query) == "" {
			return codex.ToolResponse{}, errors.New("query is required")
		}
		if input.Limit <= 0 {
			input.Limit = 8
		}

		query := strings.ToLower(strings.TrimSpace(input.Query))
		specs := a.tools.Specs()
		lines := make([]string, 0, input.Limit)
		for _, spec := range specs {
			external := codex.ExternalToolName(spec.Name)
			haystack := strings.ToLower(spec.Name + "\n" + external + "\n" + spec.Description)
			if !strings.Contains(haystack, query) {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", external, spec.Description))
			if len(lines) >= input.Limit {
				break
			}
		}
		if len(lines) == 0 {
			return textTool("no matching tools"), nil
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "tools.describe",
		Description: "単一 tool の説明と引数を詳しく見る",
		InputSchema: objectSchema(fieldSchema("name", "string", "internal 名または external 名")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return codex.ToolResponse{}, errors.New("name is required")
		}
		specs := a.tools.Specs()
		for _, spec := range specs {
			external := codex.ExternalToolName(spec.Name)
			if spec.Name != name && external != name {
				continue
			}
			return textTool(fmt.Sprintf("name=%s\ninternal_name=%s\ndescription=%s\nargs=%s", external, spec.Name, spec.Description, renderToolArguments(spec.InputSchema))), nil
		}
		return textTool("tool not found"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "system.now",
		Description: "現在時刻、タイムゾーン、曜日を確認する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		now := time.Now().In(a.loc)
		return textTool(fmt.Sprintf("now=%s\ntimezone=%s\nweekday=%s", now.Format(time.RFC3339), a.loc.String(), now.Weekday())), nil
	})
}
