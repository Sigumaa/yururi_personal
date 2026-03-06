package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func TestStoreMessageFactAndJobLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.SaveMessage(ctx, Message{
		ID:          "m1",
		ChannelID:   "c1",
		ChannelName: "monologue",
		AuthorID:    "u1",
		AuthorName:  "owner",
		Content:     "codex stable release を見張ってほしい",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save message: %v", err)
	}

	hits, err := store.SearchMessages(ctx, "stable", 10)
	if err != nil {
		t.Fatalf("search messages: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one message hit")
	}

	if err := store.UpsertFact(ctx, Fact{
		Kind:            "preference",
		Key:             "watch/codex",
		Value:           "enabled",
		SourceMessageID: "m1",
	}); err != nil {
		t.Fatalf("upsert fact: %v", err)
	}
	facts, err := store.SearchFacts(ctx, "codex", 10)
	if err != nil {
		t.Fatalf("search facts: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected fact hit")
	}

	job := jobs.NewJob("j1", "codex_release_watch", "watch", "channel", "1h", map[string]any{"repo": "openai/codex"})
	job.NextRunAt = now.Add(-time.Minute)
	if err := store.UpsertJob(ctx, job); err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	due, err := store.DueJobs(ctx, now, 10)
	if err != nil {
		t.Fatalf("due jobs: %v", err)
	}
	if len(due) != 1 || due[0].ID != "j1" {
		t.Fatalf("unexpected due jobs: %#v", due)
	}
}

func TestMessagesBetween(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	start := time.Now().UTC().Add(-2 * time.Hour)
	for i, createdAt := range []time.Time{start, start.Add(time.Hour), start.Add(3 * time.Hour)} {
		if err := store.SaveMessage(ctx, Message{
			ID:          string(rune('a' + i)),
			ChannelID:   "c1",
			ChannelName: "chat",
			AuthorID:    "u1",
			AuthorName:  "owner",
			Content:     "note",
			CreatedAt:   createdAt,
		}); err != nil {
			t.Fatalf("save message %d: %v", i, err)
		}
	}
	got, err := store.MessagesBetween(ctx, start, start.Add(2*time.Hour), 10)
	if err != nil {
		t.Fatalf("messages between: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages in range, got %d", len(got))
	}
}

func TestLatestChannelIDForAuthor(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	base := time.Now().UTC()
	for i, channelID := range []string{"c1", "c2"} {
		if err := store.SaveMessage(ctx, Message{
			ID:          string(rune('m' + i)),
			ChannelID:   channelID,
			ChannelName: "chat",
			AuthorID:    "owner",
			AuthorName:  "owner",
			Content:     "note",
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("save message %d: %v", i, err)
		}
	}

	got, ok, err := store.LatestChannelIDForAuthor(ctx, "owner")
	if err != nil {
		t.Fatalf("latest channel: %v", err)
	}
	if !ok || got != "c2" {
		t.Fatalf("unexpected latest channel: ok=%v got=%s", ok, got)
	}
}

func TestListDeleteFactsAndChannelActivity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	for i, channelID := range []string{"c1", "c1", "c2"} {
		if err := store.SaveMessage(ctx, Message{
			ID:          fmt.Sprintf("m%d", i),
			ChannelID:   channelID,
			ChannelName: map[string]string{"c1": "general", "c2": "notes"}[channelID],
			AuthorID:    "u1",
			AuthorName:  "owner",
			Content:     "note",
			CreatedAt:   now.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("save message %d: %v", i, err)
		}
	}

	if err := store.UpsertFact(ctx, Fact{Kind: "preference", Key: "tone", Value: "soft"}); err != nil {
		t.Fatalf("upsert fact tone: %v", err)
	}
	if err := store.UpsertFact(ctx, Fact{Kind: "preference", Key: "tempo", Value: "quiet"}); err != nil {
		t.Fatalf("upsert fact tempo: %v", err)
	}

	facts, err := store.ListFacts(ctx, "preference", 10)
	if err != nil {
		t.Fatalf("list facts: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if err := store.DeleteFact(ctx, "preference", "tone"); err != nil {
		t.Fatalf("delete fact: %v", err)
	}
	facts, err = store.ListFacts(ctx, "preference", 10)
	if err != nil {
		t.Fatalf("list facts after delete: %v", err)
	}
	if len(facts) != 1 || facts[0].Key != "tempo" {
		t.Fatalf("unexpected facts after delete: %#v", facts)
	}

	activity, err := store.ChannelActivitySince(ctx, now.Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("channel activity: %v", err)
	}
	if len(activity) != 2 {
		t.Fatalf("expected 2 activity rows, got %d", len(activity))
	}
	if activity[0].ChannelID != "c1" || activity[0].MessageCount != 2 {
		t.Fatalf("unexpected top activity row: %#v", activity[0])
	}
}

func TestListChannelProfiles(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "yururi.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.UpsertChannelProfile(ctx, ChannelProfile{
		ChannelID:           "c1",
		Name:                "general",
		Kind:                "conversation",
		ReplyAggressiveness: 0.7,
		AutonomyLevel:       0.6,
		SummaryCadence:      "daily",
	}); err != nil {
		t.Fatalf("upsert channel profile: %v", err)
	}

	profiles, err := store.ListChannelProfiles(ctx)
	if err != nil {
		t.Fatalf("list channel profiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ChannelID != "c1" {
		t.Fatalf("unexpected profiles: %#v", profiles)
	}
}
