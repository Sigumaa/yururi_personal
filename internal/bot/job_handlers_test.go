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
	"github.com/Sigumaa/yururi_personal/internal/review"
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
	decisionPrompt := review.DecisionPrompt(noReplyToken,
		[]memory.Fact{{Kind: "decision", Key: "tone", Value: "少し短めにする"}},
		[]memory.Message{{ChannelName: "general", Content: "最近は短く返してほしい", CreatedAt: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC)}},
	)
	if !strings.Contains(decisionPrompt, "recent decisions") || !strings.Contains(decisionPrompt, "tone") {
		t.Fatalf("unexpected decision prompt: %s", decisionPrompt)
	}

	improvementPrompt := review.SelfImprovementPrompt(noReplyToken,
		[]memory.Fact{{Kind: "automation_candidate", Key: "space", Value: "空間整理を楽にしたい"}},
		[]memory.Summary{{Content: "前置きだけで止まらないほうがよい"}},
		[]memory.Summary{{Content: "tool call が安定してきた"}},
	)
	for _, want := range []string{"automation candidates", "space", "前置きだけで止まらない", "tool call が安定"} {
		if !strings.Contains(improvementPrompt, want) {
			t.Fatalf("expected %q in improvement prompt, got %s", want, improvementPrompt)
		}
	}

	rolePrompt := review.ChannelRolePrompt(noReplyToken,
		[]discordsvc.Channel{{ID: "cat1", Name: "lab", Type: discordgo.ChannelTypeGuildCategory}, {ID: "c1", Name: "general", ParentID: "cat1", Type: discordgo.ChannelTypeGuildText}},
		[]memory.ChannelProfile{{ChannelID: "c1", Name: "general", Kind: "conversation", ReplyAggressiveness: 0.75, AutonomyLevel: 0.55, SummaryCadence: "daily"}},
		[]memory.ChannelActivity{{ChannelID: "c1", ChannelName: "general", MessageCount: 8, LastMessageAt: time.Now().UTC()}},
	)
	if !strings.Contains(rolePrompt, "channel role") || !strings.Contains(rolePrompt, "server snapshot") {
		t.Fatalf("unexpected role prompt: %s", rolePrompt)
	}

	curiosityPrompt := review.CuriosityPrompt(noReplyToken,
		[]memory.Fact{{Kind: "curiosity", Key: "rust-runtime", Value: "tokio 以外も気になる"}},
		[]memory.Fact{{Kind: "open_loop", Key: "agent-flow", Value: "会話しながら tool を回したい"}},
		[]memory.Message{{ChannelName: "general", Content: "そういえば別 runtime も気になる", CreatedAt: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC)}},
	)
	for _, want := range []string{"curiosities", "rust-runtime", "open loops"} {
		if !strings.Contains(curiosityPrompt, want) {
			t.Fatalf("expected %q in curiosity prompt, got %s", want, curiosityPrompt)
		}
	}

	initiativePrompt := review.InitiativePrompt(noReplyToken,
		[]memory.Fact{{Kind: "initiative", Key: "cleanup", Value: "空間整理を提案したい"}},
		[]memory.Fact{{Kind: "automation_candidate", Key: "watch", Value: "監視候補が増えている"}},
		[]memory.Fact{{Kind: "open_loop", Key: "space", Value: "整理のタイミングを見たい"}},
		[]memory.Fact{{Kind: "context_gap", Key: "sleep", Value: "生活リズムの確信が薄い"}},
	)
	for _, want := range []string{"initiatives", "cleanup", "context gaps"} {
		if !strings.Contains(initiativePrompt, want) {
			t.Fatalf("expected %q in initiative prompt, got %s", want, initiativePrompt)
		}
	}

	softReminderPrompt := review.SoftReminderPrompt(noReplyToken,
		[]memory.Fact{{Kind: "soft_reminder", Key: "cleanup", Value: "来月くらいに整理したい"}},
		[]memory.Fact{{Kind: "routine", Key: "morning", Value: "朝に Discord を見る"}},
		[]memory.Message{{ChannelName: "general", Content: "来月あたりに整理しようかな", CreatedAt: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC)}},
	)
	for _, want := range []string{"soft reminders", "cleanup", "routines"} {
		if !strings.Contains(softReminderPrompt, want) {
			t.Fatalf("expected %q in soft reminder prompt, got %s", want, softReminderPrompt)
		}
	}

	topicPrompt := review.TopicSynthesisPrompt(noReplyToken,
		[]memory.Fact{{Kind: "topic_thread", Key: "auth", Value: "OAuth と認証の断片"}},
		[]memory.Message{{ChannelName: "reading", Content: "OAuth 解説記事よかった", CreatedAt: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC)}},
		[]memory.Summary{{Content: "今週は認証まわりが増えていた"}},
	)
	for _, want := range []string{"topic threads", "auth", "recent summaries"} {
		if !strings.Contains(topicPrompt, want) {
			t.Fatalf("expected %q in topic prompt, got %s", want, topicPrompt)
		}
	}

	baselinePrompt := review.BaselinePrompt(noReplyToken,
		[]memory.Fact{{Kind: "behavior_baseline", Key: "late-night", Value: "普段は23時台に静か"}},
		[]memory.Fact{{Kind: "behavior_deviation", Key: "late-night-shift", Value: "今日は深夜も活動している"}},
		[]memory.Fact{{Kind: "routine", Key: "night", Value: "夜は静かめ"}},
		[]memory.Message{{ChannelName: "general", Content: "まだ起きてる", CreatedAt: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC)}},
	)
	for _, want := range []string{"behavior baselines", "behavior deviations", "late-night-shift"} {
		if !strings.Contains(baselinePrompt, want) {
			t.Fatalf("expected %q in baseline prompt, got %s", want, baselinePrompt)
		}
	}

	policyPrompt := review.PolicySynthesisPrompt(noReplyToken,
		[]memory.Fact{{Kind: "learned_policy", Key: "notify-lightly", Value: "軽い通知は一言で済ませる"}},
		[]memory.Fact{{Kind: "decision", Key: "tone", Value: "説明は短めにする"}},
		[]memory.Fact{{Kind: "misfire", Key: "over-reply", Value: "独り言に反応しすぎた"}},
		[]memory.Summary{{Content: "前置きだけで止まらないほうがよい"}},
	)
	for _, want := range []string{"learned policies", "notify-lightly", "recent misfires"} {
		if !strings.Contains(policyPrompt, want) {
			t.Fatalf("expected %q in policy prompt, got %s", want, policyPrompt)
		}
	}

	workspacePrompt := review.WorkspacePrompt(noReplyToken,
		[]memory.Fact{{Kind: "workspace_note", Key: "workshop-draft", Value: "作業用チャンネルをどう作るかの下書き"}},
		[]memory.Fact{{Kind: "initiative", Key: "workshop", Value: "作業場所の候補を見たい"}},
		[]memory.Fact{{Kind: "topic_thread", Key: "auth", Value: "認証まわりの断片が増えている"}},
		[]memory.Message{{ChannelName: "general", Content: "作業場所を分けたいかも", CreatedAt: time.Date(2026, 3, 7, 1, 0, 0, 0, time.UTC)}},
	)
	for _, want := range []string{"workspace notes", "workshop-draft", "topic threads"} {
		if !strings.Contains(workspacePrompt, want) {
			t.Fatalf("expected %q in workspace prompt, got %s", want, workspacePrompt)
		}
	}

	boundaryPrompt := review.ProposalBoundaryPrompt(noReplyToken,
		[]memory.Fact{{Kind: "proposal_boundary", Key: "space-boundary", Value: "整理案は先に作って、変更は提案に留める"}},
		[]memory.Fact{{Kind: "initiative", Key: "cleanup", Value: "空間整理をしたい"}},
		[]memory.Fact{{Kind: "decision", Key: "autonomy-mode", Value: "小さな整理は今やる"}},
		[]memory.Fact{{Kind: "misfire", Key: "too-bold", Value: "勝手にやりすぎた"}},
		[]memory.Fact{{Kind: "context_gap", Key: "scope", Value: "どこまで触ってよいか迷う"}},
	)
	for _, want := range []string{"proposal boundaries", "space-boundary", "context gaps"} {
		if !strings.Contains(boundaryPrompt, want) {
			t.Fatalf("expected %q in proposal boundary prompt, got %s", want, boundaryPrompt)
		}
	}
}
