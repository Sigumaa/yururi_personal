package bot

import (
	"strings"
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func TestInstructionsMentionPersonaAndContextDocs(t *testing.T) {
	if !strings.Contains(baseInstructions(), "女子大生メイド") {
		t.Fatalf("expected base instructions to mention persona, got %s", baseInstructions())
	}
	dev := developerInstructions()
	if !strings.Contains(dev, "workspace/context/*.md") {
		t.Fatalf("expected developer instructions to mention context docs, got %s", dev)
	}
	if !strings.Contains(dev, "未記載の能力をできる前提で話さない") {
		t.Fatalf("expected developer instructions to guard unsupported powers, got %s", dev)
	}
	if !strings.Contains(dev, "確認なく実行してよい") {
		t.Fatalf("expected developer instructions to prefer act-first, got %s", dev)
	}
	if !strings.Contains(dev, "必要なら会話の途中で複数回メッセージを送ってよい") {
		t.Fatalf("expected developer instructions to allow multi-message progress, got %s", dev)
	}
}

func TestRenderMessageForPromptIncludesAttachments(t *testing.T) {
	msg := memory.Message{
		Content: "見てほしいです",
		Metadata: map[string]any{
			"attachments": []string{"https://example.com/image.png"},
		},
	}

	got := renderMessageForPrompt(msg)
	if !strings.Contains(got, "attachments:") || !strings.Contains(got, "https://example.com/image.png") {
		t.Fatalf("expected attachments in prompt rendering, got %s", got)
	}
}

func TestBuildBackgroundTaskPromptForcesExecution(t *testing.T) {
	prompt := buildBackgroundTaskPrompt(jobs.Job{
		ID:        "job-1",
		Title:     "tools quick check",
		ChannelID: "channel-1",
	}, "サーバー俯瞰と job 一覧を確認して短くまとめる")

	if !strings.Contains(prompt, "tool を使わずに、できない・接続できない・確認できないと決めつけない") {
		t.Fatalf("expected tool-first guard, got %s", prompt)
	}
	if !strings.Contains(prompt, "discord.describe_server") {
		t.Fatalf("expected discord tool hint, got %s", prompt)
	}
	if !strings.Contains(prompt, "サーバー俯瞰と job 一覧を確認して短くまとめる") {
		t.Fatalf("expected original task prompt, got %s", prompt)
	}
}

func TestPlannerPromptPrefersImmediateExecutionOverJobs(t *testing.T) {
	prompt := buildPlannerPrompt(
		memory.Message{
			ChannelID:   "c-1",
			ChannelName: "general",
			AuthorID:    "u-1",
			AuthorName:  "shiyui",
			Content:     "できることを確認して",
		},
		memory.ChannelProfile{Name: "general", Kind: "conversation", ReplyAggressiveness: 0.8, AutonomyLevel: 0.8},
		nil,
		nil,
		nil,
		"<@bot>",
	)

	if !strings.Contains(prompt, "その場で終わる確認、俯瞰、読取り、軽い編集は、job にせず今この turn で完了させる") {
		t.Fatalf("expected prompt to avoid unnecessary jobs, got %s", prompt)
	}
	if !strings.Contains(prompt, "discord.send_message を使って会話の途中で複数回話してよい") {
		t.Fatalf("expected prompt to allow multiple visible updates, got %s", prompt)
	}
	if !strings.Contains(prompt, "actions に announcement_text を入れると、実行の前に自然な一言を挟める") {
		t.Fatalf("expected prompt to mention action announcement, got %s", prompt)
	}
}
