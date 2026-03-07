package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func (a *App) registerWebTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "web.fetch_url",
		Description: "URL を取得して title と本文抜粋を読む",
		InputSchema: objectSchema(
			fieldSchema("url", "string", "取得対象 URL"),
			fieldSchema("max_chars", "integer", "本文の最大文字数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			URL      string `json:"url"`
			MaxChars int    `json:"max_chars"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.URL) == "" {
			return codex.ToolResponse{}, errors.New("url is required")
		}
		snapshot, err := a.fetchURLSnapshot(ctx, input.URL, input.MaxChars)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{
			fmt.Sprintf("title=%s", snapshot.Title),
			fmt.Sprintf("status=%d", snapshot.StatusCode),
			fmt.Sprintf("content_type=%s", snapshot.ContentType),
			fmt.Sprintf("final_url=%s", snapshot.FinalURL),
			fmt.Sprintf("text=%s", snapshot.Text),
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
}

func (a *App) registerMediaTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "media.load_attachments",
		Description: "画像 URL 群を会話コンテキストへ読み込み、スクリーンショットや画像添付を見られるようにする",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"urls": map[string]any{
					"type":        "array",
					"description": "画像 URL の配列",
					"items": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			URLs []string `json:"urls"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		imageInputs, notes, err := a.buildImageInputs(ctx, input.URLs)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(imageInputs) == 0 {
			return codex.ToolResponse{}, errors.New("urls are required")
		}
		items := make([]codex.ToolContentItem, 0, len(imageInputs)+1)
		prefix := codex.ToolContentItem{
			Type: "inputText",
			Text: "loaded attachments:\n" + strings.Join(notes, "\n"),
		}
		items = append(items, prefix)
		for _, inputItem := range imageInputs {
			items = append(items, codex.ToolContentItem{
				Type:     "inputImage",
				ImageURL: inputItem.URL,
			})
		}
		return codex.ToolResponse{Success: true, ContentItems: items}, nil
	})
}
