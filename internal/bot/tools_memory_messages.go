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

func (a *App) registerMemoryMessageTools(registry *codex.ToolRegistry) {
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

		messages, err := a.recentOwnerMessages(ctx, input.ChannelID, input.Query, input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
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
		routines, err := a.store.ListFacts(ctx, "routine", input.Limit)
		if err != nil {
			return codex.ToolResponse{}, err
		}
		openLoops, err := a.store.ListFacts(ctx, "open_loop", input.Limit)
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
		lines = appendOwnerMessageSection(lines, ownerMessages, a.loc)
		lines = appendFactSection(lines, "routines:", routines)
		lines = appendFactSection(lines, "open_loops:", openLoops)
		lines = appendFactSection(lines, "pending_promises:", pendingPromises)
		lines = appendFactSection(lines, "curiosities:", curiosities)
		lines = appendFactSection(lines, "agent_goals:", agentGoals)
		lines = appendFactSection(lines, "soft_reminders:", softReminders)
		lines = appendFactSection(lines, "topic_threads:", topicThreads)
		lines = appendFactSection(lines, "initiatives:", initiatives)
		lines = appendFactSection(lines, "automation_candidates:", automationCandidates)
		lines = appendFactSection(lines, "learned_policies:", learnedPolicies)
		lines = appendFactSection(lines, "workspace_notes:", workspaceNotes)
		lines = appendFactSection(lines, "proposal_boundaries:", proposalBoundaries)
		lines = appendSummarySection(lines, "reflections:", reflections, a.loc)
		lines = appendSummarySection(lines, "growth:", growth, a.loc)
		lines = appendFactSection(lines, "decisions:", decisions)
		lines = appendFactSection(lines, "context_gaps:", contextGaps)
		lines = appendFactSection(lines, "misfires:", misfires)
		lines = appendFactSection(lines, "behavior_baselines:", baselines)
		lines = appendFactSection(lines, "behavior_deviations:", deviations)
		return textTool(strings.Join(lines, "\n")), nil
	})
}

func (a *App) recentOwnerMessages(ctx context.Context, channelID string, query string, limit int) ([]memory.Message, error) {
	if strings.TrimSpace(query) == "" {
		return a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, channelID, limit)
	}

	hits, err := a.store.SearchMessages(ctx, query, max(limit*4, 20))
	if err != nil {
		return nil, err
	}

	messages := make([]memory.Message, 0, limit)
	for _, msg := range hits {
		if msg.AuthorID != a.cfg.Discord.OwnerUserID {
			continue
		}
		if channelID != "" && msg.ChannelID != channelID {
			continue
		}
		messages = append(messages, msg)
		if len(messages) >= limit {
			break
		}
	}
	return messages, nil
}

func appendOwnerMessageSection(lines []string, messages []memory.Message, loc *time.Location) []string {
	if len(messages) == 0 {
		return append(lines, "- none")
	}
	for _, msg := range messages {
		lines = append(lines, fmt.Sprintf("- [%s] %s: %s", msg.CreatedAt.In(loc).Format(time.RFC3339), msg.ChannelName, truncateText(msg.Content, 220)))
	}
	return lines
}

func appendFactSection(lines []string, title string, facts []memory.Fact) []string {
	lines = append(lines, title)
	if len(facts) == 0 {
		return append(lines, "- none")
	}
	for _, fact := range facts {
		lines = append(lines, fmt.Sprintf("- %s: %s", fact.Key, truncateText(fact.Value, 220)))
	}
	return lines
}

func appendSummarySection(lines []string, title string, summaries []memory.Summary, loc *time.Location) []string {
	lines = append(lines, title)
	if len(summaries) == 0 {
		return append(lines, "- none")
	}
	for _, summary := range summaries {
		lines = append(lines, fmt.Sprintf("- [%s] %s", summary.CreatedAt.In(loc).Format(time.RFC3339), truncateText(summary.Content, 220)))
	}
	return lines
}
