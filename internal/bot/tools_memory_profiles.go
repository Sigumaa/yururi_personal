package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (a *App) registerMemorySummaryTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.recent_summaries",
		Description: "保存済み summary を period 単位で読む",
		InputSchema: objectSchema(
			fieldSchema("period", "string", "daily, weekly, growth, wake など"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Period string `json:"period"`
			Limit  int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if strings.TrimSpace(input.Period) == "" {
			return codex.ToolResponse{}, errors.New("period is required")
		}
		if input.Limit <= 0 {
			input.Limit = 5
		}
		summaries, err := a.store.RecentSummaries(ctx, input.Period, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(summaries) == 0 {
			return textTool("no summaries"), nil
		}

		lines := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			lines = append(lines, fmt.Sprintf("- [%s] channel=%s %s", summary.CreatedAt.In(a.loc).Format(time.RFC3339), summary.ChannelID, truncateText(summary.Content, 240)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.list_notes",
		Description: "reflection, growth, daily, weekly, monthly, wake などのノートを period ごとに読む",
		InputSchema: objectSchema(
			fieldSchema("period", "string", "reflection, growth, daily, weekly, monthly, wake など"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Period string `json:"period"`
			Limit  int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if strings.TrimSpace(input.Period) == "" {
			return codex.ToolResponse{}, errors.New("period is required")
		}
		if input.Limit <= 0 {
			input.Limit = 10
		}
		summaries, err := a.store.RecentSummaries(ctx, input.Period, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(summaries) == 0 {
			return textTool("no notes"), nil
		}

		lines := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			lines = append(lines, fmt.Sprintf("- [%s] channel=%s %s", summary.CreatedAt.In(a.loc).Format(time.RFC3339), summary.ChannelID, truncateText(summary.Content, 240)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_reflection",
		Description: "会話や状況の振り返りを reflection として保存する",
		InputSchema: objectSchema(
			fieldSchema("content", "string", "振り返り内容"),
			fieldSchema("channel_id", "string", "関連チャンネル ID"),
			fieldSchema("period", "string", "summary period。省略時は reflection"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Content   string `json:"content"`
			ChannelID string `json:"channel_id"`
			Period    string `json:"period"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Content) == "" {
			return codex.ToolResponse{}, errors.New("content is required")
		}
		period := strings.TrimSpace(input.Period)
		if period == "" {
			period = "reflection"
		}
		now := time.Now().UTC()
		if err := a.store.SaveSummary(ctx, memory.Summary{
			Period:    period,
			ChannelID: input.ChannelID,
			Content:   input.Content,
			StartsAt:  now,
			EndsAt:    now,
			CreatedAt: now,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.write_growth_log",
		Description: "成長ログを保存する",
		InputSchema: objectSchema(
			fieldSchema("content", "string", "成長内容"),
			fieldSchema("channel_id", "string", "関連チャンネル ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Content   string `json:"content"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Content) == "" {
			return codex.ToolResponse{}, errors.New("content is required")
		}
		now := time.Now().UTC()
		if err := a.store.SaveSummary(ctx, memory.Summary{
			Period:    "growth",
			ChannelID: input.ChannelID,
			Content:   input.Content,
			StartsAt:  now,
			EndsAt:    now,
			CreatedAt: now,
		}); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("saved"), nil
	})
}

func (a *App) registerMemoryProfileTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "memory.list_channel_profiles",
		Description: "学習済みの channel profile を一覧する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(profiles) == 0 {
			return textTool("no channel profiles"), nil
		}

		lines := make([]string, 0, len(profiles))
		for _, profile := range profiles {
			lines = append(lines, fmt.Sprintf("- %s id=%s kind=%s reply=%.2f autonomy=%.2f cadence=%s", profile.Name, profile.ChannelID, profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel, profile.SummaryCadence))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.set_channel_profile",
		Description: "channel profile を更新して振る舞いの基準を整える",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("name", "string", "チャンネル名"),
			fieldSchema("kind", "string", "conversation, monologue, notifications など"),
			fieldSchema("reply_aggressiveness", "number", "0.0-1.0"),
			fieldSchema("autonomy_level", "number", "0.0-1.0"),
			fieldSchema("summary_cadence", "string", "daily, weekly など"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ChannelID           string  `json:"channel_id"`
			Name                string  `json:"name"`
			Kind                string  `json:"kind"`
			ReplyAggressiveness float64 `json:"reply_aggressiveness"`
			AutonomyLevel       float64 `json:"autonomy_level"`
			SummaryCadence      string  `json:"summary_cadence"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("channel_id is required")
		}

		profile, ok, err := a.store.GetChannelProfile(ctx, input.ChannelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			profile = memory.ChannelProfile{ChannelID: input.ChannelID}
		}
		if input.Name != "" {
			profile.Name = input.Name
		}
		if input.Kind != "" {
			profile.Kind = input.Kind
		}
		if input.ReplyAggressiveness > 0 {
			profile.ReplyAggressiveness = input.ReplyAggressiveness
		}
		if input.AutonomyLevel > 0 {
			profile.AutonomyLevel = input.AutonomyLevel
		}
		if input.SummaryCadence != "" {
			profile.SummaryCadence = input.SummaryCadence
		}
		if profile.Name == "" {
			profile.Name = input.ChannelID
		}
		if profile.Kind == "" {
			profile.Kind = "conversation"
		}
		if profile.ReplyAggressiveness == 0 {
			profile.ReplyAggressiveness = 0.75
		}
		if profile.AutonomyLevel == 0 {
			profile.AutonomyLevel = 0.55
		}
		if profile.SummaryCadence == "" {
			profile.SummaryCadence = "daily"
		}
		if err := a.store.UpsertChannelProfile(ctx, profile); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("ok"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.channel_activity",
		Description: "最近のチャンネル活動量を俯瞰する",
		InputSchema: objectSchema(
			fieldSchema("since_hours", "integer", "何時間ぶん見るか"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			SinceHours int `json:"since_hours"`
			Limit      int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if len(activity) == 0 {
			return textTool("no recent activity"), nil
		}

		lines := make([]string, 0, len(activity))
		for _, item := range activity {
			lines = append(lines, fmt.Sprintf("- %s id=%s messages=%d last=%s", item.ChannelName, item.ChannelID, item.MessageCount, item.LastMessageAt.In(a.loc).Format(time.RFC3339)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
}
