package bot

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/config"
	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func TestHandleReminderJobSendsMessage(t *testing.T) {
	discord := &discordStub{}
	app := &App{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		discord: discord,
	}

	result, err := app.handleReminderJob(context.Background(), jobs.Job{
		ID:        "reminder-1",
		Title:     "ping",
		ChannelID: "c-1",
		Payload: map[string]any{
			"message": "朝ですよ。",
		},
	})
	if err != nil {
		t.Fatalf("handleReminderJob: %v", err)
	}
	if !result.Done || !result.AlreadyNotified {
		t.Fatalf("unexpected result: %#v", result)
	}
	if discord.sentChannel != "c-1" || discord.sentContent != "朝ですよ。" {
		t.Fatalf("unexpected reminder send: %#v", discord.sentMessages)
	}
}

func TestHandleSpaceReviewJobSendsReport(t *testing.T) {
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
		t.Fatalf("save message: %v", err)
	}
	if err := store.UpsertChannelProfile(ctx, memory.ChannelProfile{
		ChannelID:           "quiet-1",
		Name:                "quiet-room",
		Kind:                "conversation",
		ReplyAggressiveness: 0.7,
		AutonomyLevel:       0.5,
		SummaryCadence:      "daily",
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}

	discord := &discordStub{
		channels: []discordsvc.Channel{
			{ID: "cat-1", Name: "lab", Type: discordgo.ChannelTypeGuildCategory},
			{ID: "root-active", Name: "general", Type: discordgo.ChannelTypeGuildText},
			{ID: "quiet-1", Name: "quiet-room", ParentID: "cat-1", Type: discordgo.ChannelTypeGuildText},
			{ID: "no-profile", Name: "loose-notes", ParentID: "cat-1", Type: discordgo.ChannelTypeGuildText},
			{ID: "empty-cat", Name: "archive", Type: discordgo.ChannelTypeGuildCategory},
		},
	}
	app := &App{
		cfg:     config.Config{Discord: config.DiscordConfig{GuildID: "g1"}},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		loc:     time.UTC,
		store:   store,
		discord: discord,
	}

	result, err := app.handleSpaceReviewJob(ctx, jobs.Job{
		ID:           "space-review-1",
		Kind:         "space_review",
		Title:        "space review",
		ChannelID:    "c-1",
		ScheduleExpr: "24h",
		Payload: map[string]any{
			"since_hours": 72,
		},
	})
	if err != nil {
		t.Fatalf("handleSpaceReviewJob: %v", err)
	}
	if result.Done {
		t.Fatalf("space review should remain scheduled: %#v", result)
	}
	if !result.AlreadyNotified {
		t.Fatalf("expected already_notified result: %#v", result)
	}
	if len(discord.sentMessages) != 1 {
		t.Fatalf("expected one message, got %#v", discord.sentMessages)
	}
	if !strings.Contains(discord.sentMessages[0].Content, "channels_missing_profile:") || !strings.Contains(discord.sentMessages[0].Content, "quiet_profiled_channels:") {
		t.Fatalf("unexpected space review report: %s", discord.sentMessages[0].Content)
	}
}

func TestReviewPromptBuilders(t *testing.T) {
	decisionPrompt := buildDecisionReviewPrompt(
		[]memory.Fact{{Kind: "decision", Key: "tone", Value: "少し短めにする"}},
		[]memory.Message{{ChannelName: "general", Content: "最近は短く返してほしい", CreatedAt: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC)}},
	)
	if !strings.Contains(decisionPrompt, "recent decisions") || !strings.Contains(decisionPrompt, "tone") {
		t.Fatalf("unexpected decision prompt: %s", decisionPrompt)
	}

	improvementPrompt := buildSelfImprovementReviewPrompt(
		[]memory.Fact{{Kind: "automation_candidate", Key: "space", Value: "空間整理を楽にしたい"}},
		[]memory.Summary{{Content: "前置きだけで止まらないほうがよい"}},
		[]memory.Summary{{Content: "tool call が安定してきた"}},
	)
	for _, want := range []string{"automation candidates", "space", "前置きだけで止まらない", "tool call が安定"} {
		if !strings.Contains(improvementPrompt, want) {
			t.Fatalf("expected %q in improvement prompt, got %s", want, improvementPrompt)
		}
	}

	rolePrompt := buildChannelRoleReviewPrompt(
		[]discordsvc.Channel{{ID: "cat1", Name: "lab", Type: discordgo.ChannelTypeGuildCategory}, {ID: "c1", Name: "general", ParentID: "cat1", Type: discordgo.ChannelTypeGuildText}},
		[]memory.ChannelProfile{{ChannelID: "c1", Name: "general", Kind: "conversation", ReplyAggressiveness: 0.75, AutonomyLevel: 0.55, SummaryCadence: "daily"}},
		[]memory.ChannelActivity{{ChannelID: "c1", ChannelName: "general", MessageCount: 8, LastMessageAt: time.Now().UTC()}},
	)
	if !strings.Contains(rolePrompt, "channel role") || !strings.Contains(rolePrompt, "server snapshot") {
		t.Fatalf("unexpected role prompt: %s", rolePrompt)
	}
}
