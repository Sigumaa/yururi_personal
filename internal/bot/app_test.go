package bot

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/config"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func TestInteractiveContextHasNoDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Fatal("expected conversation context without deadline")
	}
}

func TestResolveChannelProfileStartsNeutral(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	app := &App{
		cfg:    config.Config{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}

	profile, err := app.resolveChannelProfile(context.Background(), "c1", "独り言")
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if profile.Kind != "conversation" {
		t.Fatalf("expected neutral conversation profile, got %#v", profile)
	}
	if profile.ReplyAggressiveness != 0.75 || profile.AutonomyLevel != 0.55 {
		t.Fatalf("unexpected default profile weights: %#v", profile)
	}
}

func TestCollectConversationFactsIncludesFoundations(t *testing.T) {
	store, err := memory.Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for _, fact := range []memory.Fact{
		{Kind: "learned_policy", Key: "notify-lightly", Value: "軽い通知は一言で済ませる"},
		{Kind: "proposal_boundary", Key: "space-boundary", Value: "整理案は先に作って、変更は提案に留める"},
		{Kind: "topic_thread", Key: "general-space", Value: "general で空間整理の話題が増えている"},
	} {
		if err := store.UpsertFact(ctx, fact); err != nil {
			t.Fatalf("upsert fact %s: %v", fact.Key, err)
		}
	}

	app := &App{
		cfg:    config.Config{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:    time.UTC,
		store:  store,
	}

	facts, err := app.collectConversationFacts(ctx, memory.Message{
		ChannelID:   "c1",
		ChannelName: "general",
		AuthorID:    "owner",
		AuthorName:  "shiyui",
		Content:     "空間整理の流れを見たい",
	}, 8)
	if err != nil {
		t.Fatalf("collect conversation facts: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected collected facts")
	}
	got := map[string]bool{}
	for _, fact := range facts {
		got[fact.Kind+"/"+fact.Key] = true
	}
	for _, want := range []string{"learned_policy/notify-lightly", "proposal_boundary/space-boundary", "topic_thread/general-space"} {
		if !got[want] {
			t.Fatalf("expected %s in collected facts, got %#v", want, facts)
		}
	}
}
