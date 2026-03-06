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
	"github.com/Sigumaa/yururi_personal/internal/memory"
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
