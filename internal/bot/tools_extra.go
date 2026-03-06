package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
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

func (a *App) registerMemoryExtraTools(registry *codex.ToolRegistry) {
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

	registry.Register(codex.ToolSpec{
		Name:        "memory.recent_owner_messages",
		Description: "オーナーの最近の発話を横断的に読み返す",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "特定チャンネルに絞る場合のチャンネル ID"),
			fieldSchema("query", "string", "検索語。省略時は時系列で新しいものを返す"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ChannelID string `json:"channel_id"`
			Query     string `json:"query"`
			Limit     int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 10
		}

		var messages []memory.Message
		if strings.TrimSpace(input.Query) == "" {
			var err error
			messages, err = a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, input.ChannelID, input.Limit)
			if err != nil {
				return codex.ToolResponse{}, err
			}
		} else {
			hits, err := a.store.SearchMessages(ctx, input.Query, max(input.Limit*4, 20))
			if err != nil {
				return codex.ToolResponse{}, err
			}
			for _, msg := range hits {
				if msg.AuthorID != a.cfg.Discord.OwnerUserID {
					continue
				}
				if input.ChannelID != "" && msg.ChannelID != input.ChannelID {
					continue
				}
				messages = append(messages, msg)
				if len(messages) >= input.Limit {
					break
				}
			}
		}

		if len(messages) == 0 {
			return textTool("no owner messages"), nil
		}
		lines := make([]string, 0, len(messages))
		for _, msg := range messages {
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", msg.CreatedAt.In(a.loc).Format(time.RFC3339), msg.ChannelName, truncateText(msg.Content, 220)))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.search_messages",
		Description: "保存済みメッセージを query と channel / author 条件で絞って検索する",
		InputSchema: objectSchema(
			fieldSchema("query", "string", "検索語"),
			fieldSchema("channel_id", "string", "対象チャンネル ID。省略可"),
			fieldSchema("author_id", "string", "対象ユーザー ID。省略可"),
			fieldSchema("limit", "integer", "取得件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Query     string `json:"query"`
			ChannelID string `json:"channel_id"`
			AuthorID  string `json:"author_id"`
			Limit     int    `json:"limit"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Query) == "" {
			return codex.ToolResponse{}, errors.New("query is required")
		}
		if input.Limit <= 0 {
			input.Limit = 10
		}
		hits, err := a.store.SearchMessages(ctx, input.Query, max(input.Limit*4, 20))
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := make([]string, 0, input.Limit)
		for _, msg := range hits {
			if strings.TrimSpace(input.ChannelID) != "" && msg.ChannelID != input.ChannelID {
				continue
			}
			if strings.TrimSpace(input.AuthorID) != "" && msg.AuthorID != input.AuthorID {
				continue
			}
			lines = append(lines, fmt.Sprintf("- [%s] %s/%s: %s", msg.CreatedAt.In(a.loc).Format(time.RFC3339), msg.ChannelName, msg.AuthorName, truncateText(msg.Content, 220)))
			if len(lines) >= input.Limit {
				break
			}
		}
		if len(lines) == 0 {
			return textTool("no matching messages"), nil
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "memory.recall_briefing",
		Description: "最近の owner 発話、routine、open loop、promise、reflection、growth、decision、gap、misfire をまとめて引く",
		InputSchema: objectSchema(fieldSchema("limit", "integer", "各セクションのおおよその件数")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 5
		}

		ownerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		openLoops, err := a.store.ListFacts(ctx, "open_loop", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		routines, err := a.store.ListFacts(ctx, "routine", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		pendingPromises, err := a.store.ListFacts(ctx, "pending_promise", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		reflections, err := a.store.RecentSummaries(ctx, "reflection", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		growth, err := a.store.RecentSummaries(ctx, "growth", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		decisions, err := a.store.ListFacts(ctx, "decision", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		contextGaps, err := a.store.ListFacts(ctx, "context_gap", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		misfires, err := a.store.ListFacts(ctx, "misfire", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}

		lines := []string{"owner_messages:"}
		if len(ownerMessages) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, msg := range ownerMessages {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", msg.CreatedAt.In(a.loc).Format(time.RFC3339), msg.ChannelName, truncateText(msg.Content, 220)))
			}
		}

		lines = append(lines, "routines:")
		if len(routines) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range routines {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "open_loops:")
		if len(openLoops) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, loop := range openLoops {
				lines = append(lines, fmt.Sprintf("- %s: %s", loop.Key, truncateText(loop.Value, 220)))
			}
		}

		lines = append(lines, "pending_promises:")
		if len(pendingPromises) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range pendingPromises {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "reflections:")
		if len(reflections) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range reflections {
				lines = append(lines, fmt.Sprintf("- [%s] %s", item.CreatedAt.In(a.loc).Format(time.RFC3339), truncateText(item.Content, 220)))
			}
		}

		lines = append(lines, "growth:")
		if len(growth) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range growth {
				lines = append(lines, fmt.Sprintf("- [%s] %s", item.CreatedAt.In(a.loc).Format(time.RFC3339), truncateText(item.Content, 220)))
			}
		}

		lines = append(lines, "decisions:")
		if len(decisions) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range decisions {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "context_gaps:")
		if len(contextGaps) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range contextGaps {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "misfires:")
		if len(misfires) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range misfires {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		return textTool(strings.Join(lines, "\n")), nil
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

func (a *App) registerJobExtraTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "jobs.get",
		Description: "単一 job の状態を見る",
		InputSchema: objectSchema(fieldSchema("id", "string", "job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		job, ok, err := a.store.GetJob(ctx, input.ID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			return textTool("job not found"), nil
		}
		return textTool(formatJob(job)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.run_now",
		Description: "既存 job の次回実行を今に寄せる",
		InputSchema: objectSchema(fieldSchema("id", "string", "job ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		job, ok, err := a.store.GetJob(ctx, input.ID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		if !ok {
			return codex.ToolResponse{}, errors.New("job not found")
		}
		now := time.Now().UTC()
		if err := a.store.UpdateJobState(ctx, job.ID, jobs.StatePending, now, "", job.LastRunAt); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool("scheduled for immediate run"), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_release_watch",
		Description: "GitHub リリース監視 job を作る",
		InputSchema: objectSchema(
			fieldSchema("repo", "string", "owner/repo"),
			fieldSchema("channel_id", "string", "通知先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Repo      string `json:"repo"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Repo == "" {
			input.Repo = "openai/codex"
		}
		if input.Schedule == "" {
			input.Schedule = a.cfg.Behavior.ReleaseWatchInterval
		}
		job := jobs.NewJob(jobID("release-watch"), "codex_release_watch", "release watch", input.ChannelID, input.Schedule, map[string]any{
			"repo": input.Repo,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_summary",
		Description: "summary / review 系の定期 job を作る",
		InputSchema: objectSchema(
			fieldSchema("kind", "string", "daily_summary, weekly_review, monthly_review, growth_log, open_loop_review"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "任意の Go duration"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Kind      string `json:"kind"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if input.Kind != "daily_summary" && input.Kind != "weekly_review" && input.Kind != "monthly_review" && input.Kind != "growth_log" && input.Kind != "open_loop_review" {
			return codex.ToolResponse{}, errors.New("unsupported summary kind")
		}
		if input.Schedule == "" {
			switch input.Kind {
			case "daily_summary", "growth_log":
				input.Schedule = "24h"
			case "weekly_review":
				input.Schedule = "168h"
			case "monthly_review":
				input.Schedule = "720h"
			case "open_loop_review":
				input.Schedule = "48h"
			}
		}
		job := jobs.NewJob(jobID(strings.ReplaceAll(input.Kind, "_", "-")), input.Kind, input.Kind, input.ChannelID, input.Schedule, nil)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_url_watch",
		Description: "任意 URL の更新監視 job を作る",
		InputSchema: objectSchema(
			fieldSchema("url", "string", "監視対象 URL"),
			fieldSchema("channel_id", "string", "通知先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			URL       string `json:"url"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.URL) == "" {
			return codex.ToolResponse{}, errors.New("url is required")
		}
		if input.Schedule == "" {
			input.Schedule = a.cfg.Behavior.ReleaseWatchInterval
		}
		job := jobs.NewJob(jobID("url-watch"), "url_watch", "url watch", input.ChannelID, input.Schedule, map[string]any{
			"url": input.URL,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_codex_task",
		Description: "バックグラウンドで Codex task を 1 回実行する job を作る",
		InputSchema: objectSchema(
			fieldSchema("title", "string", "job の表示名"),
			fieldSchema("prompt", "string", "バックグラウンドで実行する依頼文"),
			fieldSchema("channel_id", "string", "結果を返すチャンネル ID"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Title     string `json:"title"`
			Prompt    string `json:"prompt"`
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Prompt) == "" {
			return codex.ToolResponse{}, errors.New("title and prompt are required")
		}
		job := jobs.NewJob(jobID("codex-task"), "codex_background_task", input.Title, input.ChannelID, "10s", map[string]any{
			"prompt": input.Prompt,
			"goal":   input.Title,
		})
		job.NextRunAt = time.Now().UTC().Add(10 * time.Second)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_periodic_codex_task",
		Description: "Codex task を定期実行する generic job を作る",
		InputSchema: objectSchema(
			fieldSchema("title", "string", "job の表示名"),
			fieldSchema("prompt", "string", "定期的に実行する依頼文"),
			fieldSchema("channel_id", "string", "結果を返すチャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Title     string `json:"title"`
			Prompt    string `json:"prompt"`
			ChannelID string `json:"channel_id"`
			Schedule  string `json:"schedule"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Prompt) == "" {
			return codex.ToolResponse{}, errors.New("title and prompt are required")
		}
		if strings.TrimSpace(input.Schedule) == "" {
			input.Schedule = "6h"
		}
		job := jobs.NewJob(jobID("codex-periodic"), "codex_periodic_task", input.Title, input.ChannelID, input.Schedule, map[string]any{
			"prompt": input.Prompt,
			"goal":   input.Title,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_reminder",
		Description: "あとで一度だけ返す reminder / follow-up を作る",
		InputSchema: objectSchema(
			fieldSchema("title", "string", "reminder の表示名"),
			fieldSchema("message", "string", "投稿する本文"),
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("after", "string", "今からどれくらい後か。Go duration 形式"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			Title     string `json:"title"`
			Message   string `json:"message"`
			ChannelID string `json:"channel_id"`
			After     string `json:"after"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.Message) == "" || strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("message and channel_id are required")
		}
		if strings.TrimSpace(input.Title) == "" {
			input.Title = "reminder"
		}
		if strings.TrimSpace(input.After) == "" {
			input.After = "30m"
		}
		delay := mustDuration(input.After, 30*time.Minute)
		job := jobs.NewJob(jobID("reminder"), "reminder", input.Title, input.ChannelID, input.After, map[string]any{
			"content": input.Message,
		})
		job.NextRunAt = time.Now().UTC().Add(delay)
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "jobs.schedule_space_review",
		Description: "空間整理候補を定期的に見直す job を作る",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "投稿先チャンネル ID"),
			fieldSchema("schedule", "string", "Go duration 形式"),
			fieldSchema("since_hours", "integer", "何時間ぶんの活動を見るか"),
			fieldSchema("title", "string", "任意の表示名"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		var input struct {
			ChannelID  string `json:"channel_id"`
			Schedule   string `json:"schedule"`
			SinceHours int    `json:"since_hours"`
			Title      string `json:"title"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("channel_id is required")
		}
		if strings.TrimSpace(input.Schedule) == "" {
			input.Schedule = "24h"
		}
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		if strings.TrimSpace(input.Title) == "" {
			input.Title = "space review"
		}
		job := jobs.NewJob(jobID("space-review"), "space_review", input.Title, input.ChannelID, input.Schedule, map[string]any{
			"since_hours": input.SinceHours,
		})
		if err := a.store.UpsertJob(ctx, job); err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("scheduled %s", job.ID)), nil
	})
}

func (a *App) registerDiscordExtraTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "discord.self_permissions",
		Description: "現在の bot 自身が指定チャンネルで持つ主要権限を確認する",
		InputSchema: objectSchema(fieldSchema("channel_id", "string", "確認対象チャンネル ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ChannelID) == "" {
			return codex.ToolResponse{}, errors.New("channel_id is required")
		}
		snapshot, err := a.discord.SelfChannelPermissions(ctx, input.ChannelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(fmt.Sprintf("user_id=%s channel_id=%s raw=%d view_channel=%t send_messages=%t manage_channels=%t",
			snapshot.UserID,
			snapshot.ChannelID,
			snapshot.Raw,
			snapshot.ViewChannel,
			snapshot.SendMessages,
			snapshot.ManageChannels,
		)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.get_channel",
		Description: "単一チャンネルの詳細を取得する",
		InputSchema: objectSchema(fieldSchema("channel_id", "string", "対象チャンネル ID")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.GetChannel(ctx, input.ChannelID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(formatChannel(channel)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.find_channels",
		Description: "チャンネル名、topic、親カテゴリ、種別でチャンネルを探す",
		InputSchema: objectSchema(
			fieldSchema("query", "string", "チャンネル名または topic の部分一致。省略可"),
			fieldSchema("parent_channel_id", "string", "親カテゴリ ID。省略可"),
			fieldSchema("kind", "string", "text または category。省略可"),
			fieldSchema("limit", "integer", "返す件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			Query           string `json:"query"`
			ParentChannelID string `json:"parent_channel_id"`
			Kind            string `json:"kind"`
			Limit           int    `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.Limit <= 0 {
			input.Limit = 12
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		query := strings.ToLower(strings.TrimSpace(input.Query))
		kind := strings.ToLower(strings.TrimSpace(input.Kind))
		lines := make([]string, 0, input.Limit)
		for _, channel := range channels {
			if strings.TrimSpace(input.ParentChannelID) != "" && channel.ParentID != input.ParentChannelID {
				continue
			}
			if query != "" {
				haystack := strings.ToLower(channel.Name + "\n" + channel.Topic)
				if !strings.Contains(haystack, query) {
					continue
				}
			}
			switch kind {
			case "text":
				if channel.Type != discordgo.ChannelTypeGuildText {
					continue
				}
			case "category":
				if channel.Type != discordgo.ChannelTypeGuildCategory {
					continue
				}
			}
			lines = append(lines, "- "+formatChannel(channel))
			if len(lines) >= input.Limit {
				break
			}
		}
		if len(lines) == 0 {
			return textTool("no matching channels"), nil
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.rename_channel",
		Description: "チャンネル名を変更する",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("name", "string", "新しいチャンネル名"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Name      string `json:"name"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.RenameChannel(ctx, input.ChannelID, sanitizeChannelName(input.Name))
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(formatChannel(channel)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.set_channel_topic",
		Description: "チャンネルの topic を変更する",
		InputSchema: objectSchema(
			fieldSchema("channel_id", "string", "対象チャンネル ID"),
			fieldSchema("topic", "string", "新しい topic"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelID string `json:"channel_id"`
			Topic     string `json:"topic"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		channel, err := a.discord.SetChannelTopic(ctx, input.ChannelID, input.Topic)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(formatChannel(channel)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.describe_server",
		Description: "カテゴリ構造、channel profile、最近の活動量をまとめて俯瞰する",
		InputSchema: objectSchema(fieldSchema("since_hours", "integer", "最近の活動を見る時間幅")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			SinceHours int `json:"since_hours"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), 64)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeServer(channels, profiles, activity, a.loc)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.ensure_space",
		Description: "カテゴリと配下チャンネル群を一度に整備する",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"category_name": map[string]any{"type": "string", "description": "作成または再利用するカテゴリ名"},
				"channels": map[string]any{
					"type":        "array",
					"description": "作成または再利用するチャンネル一覧",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"name":  map[string]any{"type": "string"},
							"topic": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			CategoryName string `json:"category_name"`
			Channels     []struct {
				Name  string `json:"name"`
				Topic string `json:"topic"`
			} `json:"channels"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.CategoryName) == "" {
			return codex.ToolResponse{}, errors.New("category_name is required")
		}
		category, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, input.CategoryName)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{fmt.Sprintf("category %s (%s)", category.Name, category.ID)}
		for _, item := range input.Channels {
			if strings.TrimSpace(item.Name) == "" {
				continue
			}
			channel, err := a.discord.EnsureTextChannel(ctx, a.cfg.Discord.GuildID, discordsvc.ChannelSpec{
				Name:     sanitizeChannelName(item.Name),
				Topic:    item.Topic,
				ParentID: category.ID,
			})
			if err != nil {
				return codex.ToolResponse{}, err
			}
			lines = append(lines, fmt.Sprintf("- %s (%s)", channel.Name, channel.ID))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.move_channels_batch",
		Description: "複数チャンネルを一括で同じカテゴリへ移動する",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"parent_channel_id": map[string]any{"type": "string"},
				"channel_ids": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ParentChannelID string   `json:"parent_channel_id"`
			ChannelIDs      []string `json:"channel_ids"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if strings.TrimSpace(input.ParentChannelID) == "" || len(input.ChannelIDs) == 0 {
			return codex.ToolResponse{}, errors.New("parent_channel_id and channel_ids are required")
		}
		lines := make([]string, 0, len(input.ChannelIDs))
		for _, channelID := range input.ChannelIDs {
			if strings.TrimSpace(channelID) == "" {
				continue
			}
			if err := a.discord.MoveChannel(ctx, channelID, input.ParentChannelID); err != nil {
				return codex.ToolResponse{}, err
			}
			lines = append(lines, fmt.Sprintf("- moved %s", channelID))
		}
		if len(lines) == 0 {
			return textTool("no channels moved"), nil
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.archive_channels",
		Description: "チャンネル群を archive カテゴリへまとめて移動し、必要なら名前に prefix を付ける",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"channel_ids": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
				},
				"archive_category_name": map[string]any{"type": "string"},
				"rename_prefix":         map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			ChannelIDs          []string `json:"channel_ids"`
			ArchiveCategoryName string   `json:"archive_category_name"`
			RenamePrefix        string   `json:"rename_prefix"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return codex.ToolResponse{}, err
		}
		if len(input.ChannelIDs) == 0 {
			return codex.ToolResponse{}, errors.New("channel_ids are required")
		}
		if strings.TrimSpace(input.ArchiveCategoryName) == "" {
			input.ArchiveCategoryName = "archive"
		}
		category, err := a.discord.EnsureCategory(ctx, a.cfg.Discord.GuildID, input.ArchiveCategoryName)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		lines := []string{fmt.Sprintf("archive %s (%s)", category.Name, category.ID)}
		for _, channelID := range input.ChannelIDs {
			channel, err := a.discord.GetChannel(ctx, channelID)
			if err != nil {
				return codex.ToolResponse{}, err
			}
			if err := a.discord.MoveChannel(ctx, channelID, category.ID); err != nil {
				return codex.ToolResponse{}, err
			}
			if strings.TrimSpace(input.RenamePrefix) != "" && !strings.HasPrefix(channel.Name, input.RenamePrefix) {
				channel, err = a.discord.RenameChannel(ctx, channelID, sanitizeChannelName(input.RenamePrefix+"-"+channel.Name))
				if err != nil {
					return codex.ToolResponse{}, err
				}
			}
			lines = append(lines, fmt.Sprintf("- archived %s (%s)", channel.Name, channel.ID))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.describe_idle_channels",
		Description: "最近使われていないチャンネルを活動量と profile つきで俯瞰する",
		InputSchema: objectSchema(
			fieldSchema("since_hours", "integer", "何時間動きがなければ idle とみなすか"),
			fieldSchema("limit", "integer", "返す件数"),
		),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			SinceHours int `json:"since_hours"`
			Limit      int `json:"limit"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		if input.Limit <= 0 {
			input.Limit = 12
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), 256)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeIdleChannels(channels, profiles, activity, input.Limit)), nil
	})

	registry.Register(codex.ToolSpec{
		Name:        "discord.describe_space_candidates",
		Description: "空間整理の候補を root/idle/profile 観点で俯瞰する",
		InputSchema: objectSchema(fieldSchema("since_hours", "integer", "最近の活動を見る時間幅")),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		if a.discord == nil {
			return codex.ToolResponse{}, errors.New("discord is not connected")
		}
		var input struct {
			SinceHours int `json:"since_hours"`
		}
		_ = json.Unmarshal(raw, &input)
		if input.SinceHours <= 0 {
			input.SinceHours = 168
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(input.SinceHours)*time.Hour), 256)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		return textTool(describeSpaceCandidates(channels, profiles, activity, a.loc)), nil
	})
}

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
		imageInputs, notes := a.buildImageInputs(ctx, input.URLs)
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

func formatJob(job jobs.Job) string {
	return fmt.Sprintf("id=%s kind=%s state=%s channel=%s schedule=%s next=%s last_error=%s payload=%s",
		job.ID,
		job.Kind,
		job.State,
		job.ChannelID,
		job.ScheduleExpr,
		job.NextRunAt.Format(time.RFC3339),
		job.LastError,
		formatMap(job.Payload),
	)
}

func formatChannel(channel discordsvc.Channel) string {
	return fmt.Sprintf("id=%s name=%s type=%d parent=%s position=%d topic=%s",
		channel.ID,
		channel.Name,
		channel.Type,
		channel.ParentID,
		channel.Position,
		channel.Topic,
	)
}

func describeServer(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, loc *time.Location) string {
	categories := map[string]discordsvc.Channel{}
	children := map[string][]discordsvc.Channel{}
	var roots []discordsvc.Channel
	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			categories[channel.ID] = channel
			continue
		}
		if channel.ParentID == "" {
			roots = append(roots, channel)
			continue
		}
		children[channel.ParentID] = append(children[channel.ParentID], channel)
	}

	activityByChannel := map[string]memory.ChannelActivity{}
	for _, item := range activity {
		activityByChannel[item.ChannelID] = item
	}
	profileByChannel := map[string]memory.ChannelProfile{}
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}

	lines := []string{"categories:"}
	for _, category := range channels {
		if category.Type != discordgo.ChannelTypeGuildCategory {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s id=%s", category.Name, category.ID))
		for _, child := range children[category.ID] {
			lines = append(lines, "- "+describeServerChannel(child, profileByChannel[child.ID], activityByChannel[child.ID], loc))
		}
	}
	lines = append(lines, "root_channels:")
	for _, channel := range roots {
		lines = append(lines, "- "+describeServerChannel(channel, profileByChannel[channel.ID], activityByChannel[channel.ID], loc))
	}
	lines = append(lines, "known_profiles:")
	if len(profiles) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, profile := range profiles {
			lines = append(lines, fmt.Sprintf("- %s id=%s kind=%s reply=%.2f autonomy=%.2f cadence=%s", profile.Name, profile.ChannelID, profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel, profile.SummaryCadence))
		}
	}
	return strings.Join(lines, "\n")
}

func describeServerChannel(channel discordsvc.Channel, profile memory.ChannelProfile, activity memory.ChannelActivity, loc *time.Location) string {
	parts := []string{
		fmt.Sprintf("%s id=%s type=%d", channel.Name, channel.ID, channel.Type),
	}
	if channel.Topic != "" {
		parts = append(parts, "topic="+truncateText(channel.Topic, 80))
	}
	if !activity.LastMessageAt.IsZero() {
		parts = append(parts, fmt.Sprintf("messages=%d last=%s", activity.MessageCount, activity.LastMessageAt.In(loc).Format(time.RFC3339)))
	}
	if profile.Kind != "" {
		parts = append(parts, fmt.Sprintf("profile=%s reply=%.2f autonomy=%.2f", profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel))
	}
	return strings.Join(parts, " | ")
}

func describeIdleChannels(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, limit int) string {
	active := map[string]bool{}
	for _, item := range activity {
		active[item.ChannelID] = true
	}
	profileByChannel := map[string]memory.ChannelProfile{}
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}

	lines := []string{"idle_channels:"}
	count := 0
	for _, channel := range channels {
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if active[channel.ID] {
			continue
		}
		profile := profileByChannel[channel.ID]
		line := fmt.Sprintf("- %s id=%s parent=%s", channel.Name, channel.ID, channel.ParentID)
		if profile.Kind != "" {
			line += fmt.Sprintf(" profile=%s reply=%.2f autonomy=%.2f", profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel)
		}
		lines = append(lines, line)
		count++
		if count >= limit {
			break
		}
	}
	if count == 0 {
		lines = append(lines, "- none")
	}
	return strings.Join(lines, "\n")
}

func describeSpaceCandidates(channels []discordsvc.Channel, profiles []memory.ChannelProfile, activity []memory.ChannelActivity, loc *time.Location) string {
	categoryNames := map[string]string{}
	childrenCount := map[string]int{}
	profileByChannel := map[string]memory.ChannelProfile{}
	activityByChannel := map[string]memory.ChannelActivity{}
	for _, profile := range profiles {
		profileByChannel[profile.ChannelID] = profile
	}
	for _, item := range activity {
		activityByChannel[item.ChannelID] = item
	}

	var activeRoots []string
	var missingProfiles []string
	var quietProfiled []string
	var emptyCategories []string

	for _, channel := range channels {
		if channel.Type == discordgo.ChannelTypeGuildCategory {
			categoryNames[channel.ID] = channel.Name
			continue
		}
		if channel.ParentID != "" {
			childrenCount[channel.ParentID]++
		}
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if channel.ParentID == "" {
			if item, ok := activityByChannel[channel.ID]; ok {
				activeRoots = append(activeRoots, fmt.Sprintf("- %s id=%s messages=%d last=%s", channel.Name, channel.ID, item.MessageCount, item.LastMessageAt.In(loc).Format(time.RFC3339)))
			}
		}
		if _, ok := profileByChannel[channel.ID]; !ok {
			parentName := categoryNames[channel.ParentID]
			if parentName == "" {
				parentName = "root"
			}
			missingProfiles = append(missingProfiles, fmt.Sprintf("- %s id=%s parent=%s", channel.Name, channel.ID, parentName))
			continue
		}
		if _, ok := activityByChannel[channel.ID]; !ok {
			profile := profileByChannel[channel.ID]
			quietProfiled = append(quietProfiled, fmt.Sprintf("- %s id=%s profile=%s cadence=%s", channel.Name, channel.ID, profile.Kind, profile.SummaryCadence))
		}
	}
	for categoryID, name := range categoryNames {
		if childrenCount[categoryID] == 0 {
			emptyCategories = append(emptyCategories, fmt.Sprintf("- %s id=%s", name, categoryID))
		}
	}

	lines := []string{"active_root_channels:"}
	if len(activeRoots) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, activeRoots...)
	}
	lines = append(lines, "channels_missing_profile:")
	if len(missingProfiles) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, missingProfiles...)
	}
	lines = append(lines, "quiet_profiled_channels:")
	if len(quietProfiled) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, quietProfiled...)
	}
	lines = append(lines, "empty_categories:")
	if len(emptyCategories) == 0 {
		lines = append(lines, "- none")
	} else {
		lines = append(lines, emptyCategories...)
	}
	return strings.Join(lines, "\n")
}

func formatMap(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
