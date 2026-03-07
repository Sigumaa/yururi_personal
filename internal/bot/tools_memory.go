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
		Description: "最近の owner 発話、routine、open loop、promise、curiosity、goal、soft reminder、topic、initiative、自動化候補、learned policy、workspace note、proposal boundary、reflection、growth、decision、gap、misfire、baseline をまとめて引く",
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
		curiosities, err := a.store.ListFacts(ctx, "curiosity", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		agentGoals, err := a.store.ListFacts(ctx, "agent_goal", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		softReminders, err := a.store.ListFacts(ctx, "soft_reminder", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		topicThreads, err := a.store.ListFacts(ctx, "topic_thread", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		initiatives, err := a.store.ListFacts(ctx, "initiative", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		automationCandidates, err := a.store.ListFacts(ctx, "automation_candidate", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		learnedPolicies, err := a.store.ListFacts(ctx, "learned_policy", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		workspaceNotes, err := a.store.ListFacts(ctx, "workspace_note", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		proposalBoundaries, err := a.store.ListFacts(ctx, "proposal_boundary", input.Limit)
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
		baselines, err := a.store.ListFacts(ctx, "behavior_baseline", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		deviations, err := a.store.ListFacts(ctx, "behavior_deviation", input.Limit)
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

		lines = append(lines, "curiosities:")
		if len(curiosities) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range curiosities {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "agent_goals:")
		if len(agentGoals) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range agentGoals {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "soft_reminders:")
		if len(softReminders) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range softReminders {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "topic_threads:")
		if len(topicThreads) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range topicThreads {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "initiatives:")
		if len(initiatives) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range initiatives {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "automation_candidates:")
		if len(automationCandidates) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range automationCandidates {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "learned_policies:")
		if len(learnedPolicies) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range learnedPolicies {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "workspace_notes:")
		if len(workspaceNotes) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range workspaceNotes {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "proposal_boundaries:")
		if len(proposalBoundaries) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range proposalBoundaries {
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

		lines = append(lines, "behavior_baselines:")
		if len(baselines) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range baselines {
				lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, truncateText(item.Value, 220)))
			}
		}

		lines = append(lines, "behavior_deviations:")
		if len(deviations) == 0 {
			lines = append(lines, "- none")
		} else {
			for _, item := range deviations {
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
