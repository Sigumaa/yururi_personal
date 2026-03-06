package bot

import (
	"context"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func (a *App) collectConversationFacts(ctx context.Context, msg memory.Message, limit int) ([]memory.Fact, error) {
	if limit <= 0 {
		limit = 12
	}

	var groups [][]memory.Fact
	for _, query := range []string{
		strings.TrimSpace(msg.ChannelName),
		strings.TrimSpace(msg.Content),
	} {
		if query == "" {
			continue
		}
		facts, err := a.store.SearchFacts(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		groups = append(groups, facts)
	}

	for _, kind := range []string{
		"pending_promise",
		"open_loop",
		"curiosity",
		"agent_goal",
		"soft_reminder",
		"topic_thread",
		"initiative",
		"learned_policy",
		"proposal_boundary",
		"behavior_baseline",
		"behavior_deviation",
		"decision",
		"context_gap",
		"misfire",
		"workspace_note",
	} {
		facts, err := a.store.ListFacts(ctx, kind, 2)
		if err != nil {
			return nil, err
		}
		groups = append(groups, facts)
	}

	seen := make(map[string]struct{}, limit)
	out := make([]memory.Fact, 0, limit)
	for _, group := range groups {
		for _, fact := range group {
			key := fact.Kind + "\x00" + fact.Key
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, fact)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}
