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

func TestConversationContextHasNoDeadline(t *testing.T) {
	ctx, cancel := conversationContext()
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
