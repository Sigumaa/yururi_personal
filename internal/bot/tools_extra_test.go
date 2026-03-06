package bot

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/Sigumaa/yururi_personal/internal/config"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func TestMemoryRecentOwnerMessagesTool(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	for i, msg := range []memory.Message{
		{ID: "m1", ChannelID: "c1", ChannelName: "general", AuthorID: "owner", AuthorName: "shiyui", Content: "alpha", CreatedAt: now},
		{ID: "m2", ChannelID: "c1", ChannelName: "general", AuthorID: "other", AuthorName: "other", Content: "beta", CreatedAt: now.Add(time.Minute)},
		{ID: "m3", ChannelID: "c2", ChannelName: "notes", AuthorID: "owner", AuthorName: "shiyui", Content: "gamma", CreatedAt: now.Add(2 * time.Minute)},
	} {
		if err := store.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("save message %d: %v", i, err)
		}
	}

	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerMemoryExtraTools(registry)

	response, err := registry.Call(ctx, "memory.recent_owner_messages", mustJSONRaw(t, map[string]any{
		"limit": 2,
	}))
	if err != nil {
		t.Fatalf("call recent_owner_messages: %v", err)
	}
	if !response.Success {
		t.Fatal("expected success")
	}
	if len(response.ContentItems) != 1 || !strings.Contains(response.ContentItems[0].Text, "gamma") || !strings.Contains(response.ContentItems[0].Text, "alpha") {
		t.Fatalf("unexpected tool response: %#v", response.ContentItems)
	}
	if strings.Contains(response.ContentItems[0].Text, "beta") {
		t.Fatalf("expected non-owner message to be filtered out: %#v", response.ContentItems[0])
	}
}

func TestMemoryOpenLoopReflectionGrowthAndDecisionTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerMemoryExtraTools(registry)

	if _, err := registry.Call(ctx, "memory.write_open_loop", mustJSONRaw(t, map[string]any{
		"key":   "agent-flow",
		"value": "会話しながら tool call を自然に回したい",
	})); err != nil {
		t.Fatalf("write open loop: %v", err)
	}
	listResponse, err := registry.Call(ctx, "memory.list_open_loops", mustJSONRaw(t, map[string]any{
		"limit": 10,
	}))
	if err != nil {
		t.Fatalf("list open loops: %v", err)
	}
	if !strings.Contains(listResponse.ContentItems[0].Text, "agent-flow") {
		t.Fatalf("expected open loop in list: %#v", listResponse.ContentItems[0])
	}
	if _, err := registry.Call(ctx, "memory.close_open_loop", mustJSONRaw(t, map[string]any{
		"key":        "agent-flow",
		"resolution": "write を止める制約を外した",
	})); err != nil {
		t.Fatalf("close open loop: %v", err)
	}
	loops, err := store.ListFacts(ctx, "open_loop", 10)
	if err != nil {
		t.Fatalf("list open loops from store: %v", err)
	}
	if len(loops) != 0 {
		t.Fatalf("expected closed open loop, got %#v", loops)
	}
	decisions, err := store.ListFacts(ctx, "decision", 10)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) == 0 || decisions[0].Value != "write を止める制約を外した" {
		t.Fatalf("expected resolution to be logged as decision, got %#v", decisions)
	}

	if _, err := registry.Call(ctx, "memory.write_reflection", mustJSONRaw(t, map[string]any{
		"channel_id": "c1",
		"content":    "今日は会話の流れを途中で切らないほうがよい",
	})); err != nil {
		t.Fatalf("write reflection: %v", err)
	}
	if _, err := registry.Call(ctx, "memory.write_growth_log", mustJSONRaw(t, map[string]any{
		"channel_id": "c1",
		"content":    "動的 tool call で会話しながら動ける幅が増えた",
	})); err != nil {
		t.Fatalf("write growth log: %v", err)
	}
	if _, err := registry.Call(ctx, "memory.write_decision_log", mustJSONRaw(t, map[string]any{
		"key":   "autonomy-mode",
		"value": "作業前の進捗メッセージは必須にしない",
	})); err != nil {
		t.Fatalf("write decision log: %v", err)
	}

	reflections, err := store.RecentSummaries(ctx, "reflection", 10)
	if err != nil {
		t.Fatalf("recent reflections: %v", err)
	}
	if len(reflections) != 1 || !strings.Contains(reflections[0].Content, "途中で切らない") {
		t.Fatalf("unexpected reflections: %#v", reflections)
	}
	growth, err := store.RecentSummaries(ctx, "growth", 10)
	if err != nil {
		t.Fatalf("recent growth: %v", err)
	}
	if len(growth) != 1 || !strings.Contains(growth[0].Content, "動的 tool call") {
		t.Fatalf("unexpected growth summaries: %#v", growth)
	}
	decisions, err = store.ListFacts(ctx, "decision", 10)
	if err != nil {
		t.Fatalf("list decisions after explicit write: %v", err)
	}
	var found bool
	for _, decision := range decisions {
		if decision.Key == "autonomy-mode" && strings.Contains(decision.Value, "必須にしない") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected explicit decision log entry, got %#v", decisions)
	}
}

func TestToolSearchAndDescribeTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{GuildID: "g1"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
		tools:  registry,
	}
	app.registerToolHelperTools(registry)
	app.registerDiscordExtraTools(registry)

	searchResponse, err := registry.Call(ctx, "tools.search", mustJSONRaw(t, map[string]any{
		"query": "channel",
	}))
	if err != nil {
		t.Fatalf("tools.search: %v", err)
	}
	if !strings.Contains(searchResponse.ContentItems[0].Text, "discord__get_channel") {
		t.Fatalf("expected channel tool in search results, got %#v", searchResponse.ContentItems)
	}

	describeResponse, err := registry.Call(ctx, "tools.describe", mustJSONRaw(t, map[string]any{
		"name": "discord__describe_server",
	}))
	if err != nil {
		t.Fatalf("tools.describe: %v", err)
	}
	if !strings.Contains(describeResponse.ContentItems[0].Text, "since_hours") {
		t.Fatalf("expected args in describe response, got %#v", describeResponse.ContentItems)
	}
}

func TestMemoryRecallBriefingTool(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.SaveMessage(ctx, memory.Message{
		ID:          "m1",
		ChannelID:   "c1",
		ChannelName: "general",
		AuthorID:    "owner",
		AuthorName:  "shiyui",
		Content:     "最近は自律性をもっと上げたい",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if err := store.UpsertFact(ctx, memory.Fact{Kind: "open_loop", Key: "autonomy", Value: "連続 tool call を自然に回したい"}); err != nil {
		t.Fatalf("upsert open loop: %v", err)
	}
	if err := store.UpsertFact(ctx, memory.Fact{Kind: "routine", Key: "morning", Value: "朝は先に Discord を見る"}); err != nil {
		t.Fatalf("upsert routine: %v", err)
	}
	if err := store.UpsertFact(ctx, memory.Fact{Kind: "pending_promise", Key: "space-review", Value: "空間整理の確認を返す"}); err != nil {
		t.Fatalf("upsert pending promise: %v", err)
	}
	if err := store.UpsertFact(ctx, memory.Fact{Kind: "decision", Key: "tone", Value: "溺愛寄りにする"}); err != nil {
		t.Fatalf("upsert decision: %v", err)
	}
	if err := store.UpsertFact(ctx, memory.Fact{Kind: "context_gap", Key: "sleep-schedule", Value: "就寝時間帯の確信がない"}); err != nil {
		t.Fatalf("upsert context gap: %v", err)
	}
	if err := store.UpsertFact(ctx, memory.Fact{Kind: "misfire", Key: "promise-only", Value: "前置きだけ送って止まった"}); err != nil {
		t.Fatalf("upsert misfire: %v", err)
	}
	if err := store.SaveSummary(ctx, memory.Summary{Period: "reflection", ChannelID: "c1", Content: "前置きだけで止まらないほうがよい", StartsAt: now, EndsAt: now}); err != nil {
		t.Fatalf("save reflection: %v", err)
	}
	if err := store.SaveSummary(ctx, memory.Summary{Period: "growth", ChannelID: "c1", Content: "tool search が使えるようになった", StartsAt: now, EndsAt: now}); err != nil {
		t.Fatalf("save growth: %v", err)
	}

	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerMemoryExtraTools(registry)

	response, err := registry.Call(ctx, "memory.recall_briefing", mustJSONRaw(t, map[string]any{
		"limit": 5,
	}))
	if err != nil {
		t.Fatalf("memory.recall_briefing: %v", err)
	}
	text := response.ContentItems[0].Text
	for _, want := range []string{"owner_messages:", "routines:", "open_loops:", "pending_promises:", "reflections:", "growth:", "decisions:", "context_gaps:", "misfires:", "autonomy", "溺愛寄り", "promise-only"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in briefing, got %s", want, text)
		}
	}
}

func TestJobSchedulingAndSpaceCandidateTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.SaveMessage(ctx, memory.Message{
		ID:          "m1",
		ChannelID:   "root-active",
		ChannelName: "general",
		AuthorID:    "owner",
		AuthorName:  "shiyui",
		Content:     "hello",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save active message: %v", err)
	}
	if err := store.UpsertChannelProfile(ctx, memory.ChannelProfile{
		ChannelID:           "quiet-1",
		Name:                "quiet-room",
		Kind:                "conversation",
		ReplyAggressiveness: 0.7,
		AutonomyLevel:       0.5,
		SummaryCadence:      "daily",
	}); err != nil {
		t.Fatalf("upsert quiet profile: %v", err)
	}

	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{GuildID: "g1"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
		discord: &discordStub{
			channels: []discordsvc.Channel{
				{ID: "cat-1", Name: "lab", Type: discordgo.ChannelTypeGuildCategory},
				{ID: "root-active", Name: "general", Type: discordgo.ChannelTypeGuildText},
				{ID: "quiet-1", Name: "quiet-room", ParentID: "cat-1", Type: discordgo.ChannelTypeGuildText},
				{ID: "no-profile", Name: "loose-notes", ParentID: "cat-1", Type: discordgo.ChannelTypeGuildText},
				{ID: "empty-cat", Name: "archive", Type: discordgo.ChannelTypeGuildCategory},
			},
		},
	}
	app.registerJobExtraTools(registry)
	app.registerDiscordExtraTools(registry)

	if _, err := registry.Call(ctx, "jobs.schedule_reminder", mustJSONRaw(t, map[string]any{
		"message":    "朝の声かけ",
		"channel_id": "c1",
		"after":      "15m",
	})); err != nil {
		t.Fatalf("schedule reminder: %v", err)
	}
	if _, err := registry.Call(ctx, "jobs.schedule_space_review", mustJSONRaw(t, map[string]any{
		"channel_id":  "c1",
		"schedule":    "24h",
		"since_hours": 72,
	})); err != nil {
		t.Fatalf("schedule space review: %v", err)
	}

	jobsList, err := store.DueJobs(ctx, time.Now().UTC().Add(365*24*time.Hour), 16)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	var reminderFound bool
	var spaceReviewFound bool
	for _, job := range jobsList {
		switch job.Kind {
		case "reminder":
			reminderFound = true
			if got, _ := job.Payload["content"].(string); got != "朝の声かけ" {
				t.Fatalf("unexpected reminder payload: %#v", job.Payload)
			}
		case "space_review":
			spaceReviewFound = true
			if got, _ := job.Payload["since_hours"].(float64); got != 72 {
				if gotInt, ok := job.Payload["since_hours"].(int); !ok || gotInt != 72 {
					t.Fatalf("unexpected space review payload: %#v", job.Payload)
				}
			}
		}
	}
	if !reminderFound {
		t.Fatalf("expected reminder job, got %#v", jobsList)
	}
	if !spaceReviewFound {
		t.Fatalf("expected space_review job, got %#v", jobsList)
	}

	response, err := registry.Call(ctx, "discord.describe_space_candidates", mustJSONRaw(t, map[string]any{
		"since_hours": 72,
	}))
	if err != nil {
		t.Fatalf("describe_space_candidates: %v", err)
	}
	text := response.ContentItems[0].Text
	for _, want := range []string{"active_root_channels:", "channels_missing_profile:", "quiet_profiled_channels:", "empty_categories:", "general", "loose-notes", "quiet-room", "archive"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in space candidates, got %s", want, text)
		}
	}
}

func TestJobScheduleSummarySupportsMonthlyAndReminder(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg: config.Config{
			Discord: config.DiscordConfig{OwnerUserID: "owner"},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerJobExtraTools(registry)

	if _, err := registry.Call(ctx, "jobs.schedule_summary", mustJSONRaw(t, map[string]any{
		"kind":       "monthly_review",
		"channel_id": "c-month",
	})); err != nil {
		t.Fatalf("schedule monthly review: %v", err)
	}
	if _, err := registry.Call(ctx, "jobs.schedule_reminder", mustJSONRaw(t, map[string]any{
		"title":      "morning-reminder",
		"message":    "おはようの引き継ぎです",
		"channel_id": "c-reminder",
		"after":      "2h",
	})); err != nil {
		t.Fatalf("schedule reminder: %v", err)
	}

	allJobs, err := store.DueJobs(ctx, time.Now().UTC().Add(365*24*time.Hour), 16)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}

	var monthlyFound bool
	var reminderFound bool
	for _, job := range allJobs {
		switch job.Kind {
		case "monthly_review":
			monthlyFound = true
			if job.ChannelID != "c-month" {
				t.Fatalf("unexpected monthly channel: %#v", job)
			}
		case "reminder":
			reminderFound = true
			if got, _ := job.Payload["content"].(string); got != "おはようの引き継ぎです" {
				t.Fatalf("unexpected reminder payload: %#v", job.Payload)
			}
		}
	}
	if !monthlyFound {
		t.Fatalf("expected monthly_review job, got %#v", allJobs)
	}
	if !reminderFound {
		t.Fatalf("expected reminder job, got %#v", allJobs)
	}
}

func TestMemoryListNotesTool(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.SaveSummary(ctx, memory.Summary{
		Period:    "reflection",
		ChannelID: "c1",
		Content:   "途中で止めずに最後まで流したい",
		StartsAt:  now,
		EndsAt:    now,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("save summary: %v", err)
	}

	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerMemoryExtraTools(registry)

	response, err := registry.Call(ctx, "memory.list_notes", mustJSONRaw(t, map[string]any{
		"period": "reflection",
		"limit":  5,
	}))
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if !strings.Contains(response.ContentItems[0].Text, "途中で止めずに最後まで流したい") {
		t.Fatalf("unexpected response: %#v", response.ContentItems)
	}
}

func TestDiscordFindChannelsTool(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg: config.Config{
			Discord: config.DiscordConfig{GuildID: "g1", OwnerUserID: "owner"},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
		discord: &discordStub{channels: []discordsvc.Channel{
			{ID: "cat1", Name: "test-lab", Type: 4},
			{ID: "c1", Name: "test-chat", Topic: "機能確認", ParentID: "cat1", Type: 0},
			{ID: "c2", Name: "links", Topic: "url watch", ParentID: "cat1", Type: 0},
		}},
	}
	app.registerDiscordExtraTools(registry)

	response, err := registry.Call(ctx, "discord.find_channels", mustJSONRaw(t, map[string]any{
		"query": "watch",
		"limit": 5,
	}))
	if err != nil {
		t.Fatalf("find channels: %v", err)
	}
	if !strings.Contains(response.ContentItems[0].Text, "links") {
		t.Fatalf("unexpected response: %#v", response.ContentItems)
	}
}

func TestAutomationCandidateAndReviewSchedulingTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerAutonomyTools(registry)

	if _, err := registry.Call(ctx, "memory.write_automation_candidate", mustJSONRaw(t, map[string]any{
		"key":   "channel-curation",
		"value": "空間整理の依頼が繰り返されるので補助を増やしたい",
	})); err != nil {
		t.Fatalf("write automation candidate: %v", err)
	}

	candidates, err := registry.Call(ctx, "memory.list_automation_candidates", mustJSONRaw(t, map[string]any{
		"limit": 5,
	}))
	if err != nil {
		t.Fatalf("list automation candidates: %v", err)
	}
	if !strings.Contains(candidates.ContentItems[0].Text, "channel-curation") {
		t.Fatalf("unexpected candidate response: %#v", candidates.ContentItems)
	}

	if _, err := registry.Call(ctx, "jobs.schedule_review", mustJSONRaw(t, map[string]any{
		"kind":       "channel_curation",
		"channel_id": "c-review",
		"schedule":   "48h",
	})); err != nil {
		t.Fatalf("schedule review: %v", err)
	}

	allJobs, err := store.DueJobs(ctx, time.Now().UTC().Add(365*24*time.Hour), 16)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	var found bool
	for _, job := range allJobs {
		if job.Kind == "channel_curation" && job.ChannelID == "c-review" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected channel_curation job, got %#v", allJobs)
	}
}

func TestDiscordStaleAndSpaceRefreshTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.SaveMessage(ctx, memory.Message{
		ID:          "m1",
		ChannelID:   "active-1",
		ChannelName: "general",
		AuthorID:    "owner",
		AuthorName:  "shiyui",
		Content:     "active",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if err := store.UpsertChannelProfile(ctx, memory.ChannelProfile{
		ChannelID:           "quiet-1",
		Name:                "quiet-room",
		Kind:                "monologue",
		ReplyAggressiveness: 0.2,
		AutonomyLevel:       0.8,
		SummaryCadence:      "weekly",
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}

	registry := codex.NewToolRegistry()
	app := &App{
		cfg: config.Config{
			Discord: config.DiscordConfig{GuildID: "g1", OwnerUserID: "owner"},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
		discord: &discordStub{channels: []discordsvc.Channel{
			{ID: "cat1", Name: "lab", Type: discordgo.ChannelTypeGuildCategory},
			{ID: "active-1", Name: "general", Type: discordgo.ChannelTypeGuildText},
			{ID: "quiet-1", Name: "quiet-room", ParentID: "cat1", Type: discordgo.ChannelTypeGuildText},
			{ID: "loose-1", Name: "loose-notes", Type: discordgo.ChannelTypeGuildText},
			{ID: "empty-cat", Name: "archive", Type: discordgo.ChannelTypeGuildCategory},
		}},
	}
	app.registerAutonomyTools(registry)

	stale, err := registry.Call(ctx, "discord.find_stale_channels", mustJSONRaw(t, map[string]any{
		"since_hours": 72,
	}))
	if err != nil {
		t.Fatalf("find stale channels: %v", err)
	}
	if !strings.Contains(stale.ContentItems[0].Text, "quiet-room") || !strings.Contains(stale.ContentItems[0].Text, "loose-notes") {
		t.Fatalf("unexpected stale channels: %#v", stale.ContentItems)
	}

	refresh, err := registry.Call(ctx, "discord.plan_space_refresh", mustJSONRaw(t, map[string]any{
		"since_hours": 72,
	}))
	if err != nil {
		t.Fatalf("plan space refresh: %v", err)
	}
	text := refresh.ContentItems[0].Text
	for _, want := range []string{"root text channels", "stale channels", "unprofiled channels", "quiet-room", "loose-notes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in refresh output, got %s", want, text)
		}
	}
}

func TestRoutineAndPendingPromiseTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerAutonomyTools(registry)

	if _, err := registry.Call(ctx, "memory.write_routine", mustJSONRaw(t, map[string]any{
		"key":   "morning-boot",
		"value": "朝は Discord を開いてから作業に入ることが多い",
	})); err != nil {
		t.Fatalf("write routine: %v", err)
	}
	if _, err := registry.Call(ctx, "memory.write_pending_promise", mustJSONRaw(t, map[string]any{
		"key":   "space-check",
		"value": "空間整理の動作確認をあとで見直す",
	})); err != nil {
		t.Fatalf("write pending promise: %v", err)
	}

	routines, err := registry.Call(ctx, "memory.list_routines", mustJSONRaw(t, map[string]any{"limit": 5}))
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if !strings.Contains(routines.ContentItems[0].Text, "morning-boot") {
		t.Fatalf("unexpected routines: %#v", routines.ContentItems)
	}

	promises, err := registry.Call(ctx, "memory.list_pending_promises", mustJSONRaw(t, map[string]any{"limit": 5}))
	if err != nil {
		t.Fatalf("list pending promises: %v", err)
	}
	if !strings.Contains(promises.ContentItems[0].Text, "space-check") {
		t.Fatalf("unexpected promises: %#v", promises.ContentItems)
	}

	if _, err := registry.Call(ctx, "memory.close_pending_promise", mustJSONRaw(t, map[string]any{
		"key":        "space-check",
		"resolution": "確認済みで問題なし",
	})); err != nil {
		t.Fatalf("close pending promise: %v", err)
	}
	remaining, err := store.ListFacts(ctx, "pending_promise", 10)
	if err != nil {
		t.Fatalf("list remaining promises: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no remaining promises, got %#v", remaining)
	}
}

func TestContextGapAndMisfireTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := codex.NewToolRegistry()
	app := &App{
		cfg:    config.Config{Discord: config.DiscordConfig{OwnerUserID: "owner"}},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}
	app.registerMemoryExtraTools(registry)

	if _, err := registry.Call(ctx, "memory.write_context_gap", mustJSONRaw(t, map[string]any{
		"key":   "work-mode",
		"value": "いま作業中か雑談中かの確信が弱い",
	})); err != nil {
		t.Fatalf("write context gap: %v", err)
	}
	if _, err := registry.Call(ctx, "memory.write_misfire", mustJSONRaw(t, map[string]any{
		"key":   "over-reply",
		"value": "反応しなくていい独り言に返してしまった",
	})); err != nil {
		t.Fatalf("write misfire: %v", err)
	}

	gaps, err := registry.Call(ctx, "memory.list_context_gaps", mustJSONRaw(t, map[string]any{"limit": 5}))
	if err != nil {
		t.Fatalf("list context gaps: %v", err)
	}
	if !strings.Contains(gaps.ContentItems[0].Text, "work-mode") {
		t.Fatalf("unexpected gaps: %#v", gaps.ContentItems)
	}

	misfires, err := registry.Call(ctx, "memory.list_misfires", mustJSONRaw(t, map[string]any{"limit": 5}))
	if err != nil {
		t.Fatalf("list misfires: %v", err)
	}
	if !strings.Contains(misfires.ContentItems[0].Text, "over-reply") {
		t.Fatalf("unexpected misfires: %#v", misfires.ContentItems)
	}
}

func TestExtendedReviewSchedulingAndSpaceInsightTools(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.SaveMessage(ctx, memory.Message{
		ID:          "m1",
		ChannelID:   "active-1",
		ChannelName: "general",
		AuthorID:    "owner",
		AuthorName:  "shiyui",
		Content:     "active",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save message: %v", err)
	}
	if err := store.UpsertChannelProfile(ctx, memory.ChannelProfile{
		ChannelID:           "profiled-1",
		Name:                "profiled-room",
		Kind:                "conversation",
		ReplyAggressiveness: 0.75,
		AutonomyLevel:       0.55,
		SummaryCadence:      "daily",
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}

	registry := codex.NewToolRegistry()
	app := &App{
		cfg: config.Config{
			Discord: config.DiscordConfig{GuildID: "g1", OwnerUserID: "owner"},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
		discord: &discordStub{channels: []discordsvc.Channel{
			{ID: "cat1", Name: "lab", Type: discordgo.ChannelTypeGuildCategory},
			{ID: "active-1", Name: "general", Type: discordgo.ChannelTypeGuildText},
			{ID: "unprofiled-1", Name: "notes", ParentID: "cat1", Type: discordgo.ChannelTypeGuildText},
			{ID: "profiled-1", Name: "profiled-room", ParentID: "cat1", Type: discordgo.ChannelTypeGuildText},
			{ID: "empty-cat", Name: "archive", Type: discordgo.ChannelTypeGuildCategory},
		}},
	}
	app.registerAutonomyTools(registry)

	for _, kind := range []string{"decision_review", "self_improvement_review", "channel_role_review"} {
		if _, err := registry.Call(ctx, "jobs.schedule_review", mustJSONRaw(t, map[string]any{
			"kind":       kind,
			"channel_id": "c-review",
		})); err != nil {
			t.Fatalf("schedule %s: %v", kind, err)
		}
	}

	allJobs, err := store.DueJobs(ctx, time.Now().UTC().Add(365*24*time.Hour), 16)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for _, kind := range []string{"decision_review", "self_improvement_review", "channel_role_review"} {
		var found bool
		for _, job := range allJobs {
			if job.Kind == kind {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %s job, got %#v", kind, allJobs)
		}
	}

	categoryMap, err := registry.Call(ctx, "discord.describe_category_map", mustJSONRaw(t, map[string]any{}))
	if err != nil {
		t.Fatalf("describe category map: %v", err)
	}
	if !strings.Contains(categoryMap.ContentItems[0].Text, "lab") || !strings.Contains(categoryMap.ContentItems[0].Text, "notes") {
		t.Fatalf("unexpected category map: %#v", categoryMap.ContentItems)
	}

	orphaned, err := registry.Call(ctx, "discord.find_orphan_channels", mustJSONRaw(t, map[string]any{}))
	if err != nil {
		t.Fatalf("find orphan channels: %v", err)
	}
	text := orphaned.ContentItems[0].Text
	for _, want := range []string{"general", "archive"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in orphan output, got %s", want, text)
		}
	}

	suggestions, err := registry.Call(ctx, "discord.suggest_channel_profiles", mustJSONRaw(t, map[string]any{
		"since_hours": 72,
	}))
	if err != nil {
		t.Fatalf("suggest channel profiles: %v", err)
	}
	if !strings.Contains(suggestions.ContentItems[0].Text, "unprofiled-1") && !strings.Contains(suggestions.ContentItems[0].Text, "notes") {
		t.Fatalf("unexpected profile suggestions: %#v", suggestions.ContentItems)
	}
}
