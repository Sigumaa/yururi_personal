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
	if !strings.Contains(baseInstructions(), "溺愛") {
		t.Fatalf("expected base instructions to mention 溺愛, got %s", baseInstructions())
	}
	if !strings.Contains(baseInstructions(), "デレデレ") {
		t.Fatalf("expected base instructions to mention デレデレ, got %s", baseInstructions())
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
	if !strings.Contains(dev, "前置きだけ送って止まらず") {
		t.Fatalf("expected developer instructions to discourage empty progress-only replies, got %s", dev)
	}
	if !strings.Contains(dev, "重たくなりすぎず") {
		t.Fatalf("expected developer instructions to temper doting tone, got %s", dev)
	}
	if !strings.Contains(dev, "好きがにじんでもよい") {
		t.Fatalf("expected developer instructions to allow affectionate tone, got %s", dev)
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
	if !strings.Contains(prompt, "discord__describe_server") {
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
	if !strings.Contains(prompt, "discord__send_message を使って会話の途中で複数回話してよい") {
		t.Fatalf("expected prompt to allow multiple visible updates, got %s", prompt)
	}
	if !strings.Contains(prompt, "actions に announcement_text を入れると、実行の前に自然な一言を挟める") {
		t.Fatalf("expected prompt to mention action announcement, got %s", prompt)
	}
}

func TestConversationPromptAllowsDirectToolsAndMultiReplies(t *testing.T) {
	prompt := buildConversationPrompt(
		memory.Message{
			ChannelID:   "c-1",
			ChannelName: "general",
			AuthorID:    "u-1",
			AuthorName:  "shiyui",
			Content:     "いま使える機能を見てみたい",
			Metadata: map[string]any{
				"attachments": []string{"https://example.com/image.png"},
			},
		},
		memory.ChannelProfile{Name: "general", Kind: "conversation", ReplyAggressiveness: 0.9, AutonomyLevel: 0.8},
		nil,
		nil,
		nil,
		"<@bot>",
		1,
	)

	if !strings.Contains(prompt, "その場で終わる確認、俯瞰、読取り、軽い編集は、job にせず今この turn で完了させる") {
		t.Fatalf("expected immediate execution guidance, got %s", prompt)
	}
	if !strings.Contains(prompt, "discord__send_message を使って複数回話してよい") {
		t.Fatalf("expected multi-reply guidance, got %s", prompt)
	}
	if !strings.Contains(prompt, "小さな write や整理は、前置きなしでそのまま tool を使って進めてよい") {
		t.Fatalf("expected action-first guidance, got %s", prompt)
	}
	if !strings.Contains(prompt, "前置きだけ送って止まらず") {
		t.Fatalf("expected no-progress-only guidance, got %s", prompt)
	}
	if !strings.Contains(prompt, "デレをにじませつつ") {
		t.Fatalf("expected affectionate tone guidance, got %s", prompt)
	}
	if !strings.Contains(prompt, "media__load_attachments") {
		t.Fatalf("expected attachment tool hint, got %s", prompt)
	}
	if !strings.Contains(prompt, "tools__search") || !strings.Contains(prompt, "tools__describe") {
		t.Fatalf("expected tool discovery hints, got %s", prompt)
	}
	if !strings.Contains(prompt, "current message の画像添付はこの turn にすでに載っている") {
		t.Fatalf("expected direct image input guidance, got %s", prompt)
	}
	for _, want := range []string{"curiosity", "agent goal", "soft reminder", "topic thread", "initiative", "behavior baseline", "behavior deviation"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected extended memory hint %q, got %s", want, prompt)
		}
	}
}

func TestAutonomyPulsePromptAllowsSilenceAndAction(t *testing.T) {
	prompt := buildAutonomyPulsePrompt(
		"c-1",
		"general",
		memory.PresenceSnapshot{Status: "online", Activities: []string{"Factorio"}},
		[]memory.ChannelActivity{{ChannelID: "c-1", ChannelName: "general", MessageCount: 12}},
		nil,
		[]memory.Message{{ChannelName: "general", Content: "open loop を拾ってほしい"}},
		[]memory.Fact{{Key: "agent-flow", Value: "会話しながら tool を回したい"}},
		[]memory.Fact{{Key: "runtime", Value: "tokio 以外の runtime が気になる"}},
		[]memory.Fact{{Key: "space", Value: "空間整理を続けたい"}},
		[]memory.Fact{{Key: "cleanup", Value: "来月くらいに整理したい"}},
		[]memory.Fact{{Key: "auth", Value: "OAuth の断片が増えている"}},
		[]memory.Fact{{Key: "proposal", Value: "監視専用の場所を提案したい"}},
		[]memory.Fact{{Key: "night", Value: "23時台に静かになりやすい"}},
		[]memory.Fact{{Key: "late", Value: "今日は深夜でも話している"}},
		[]memory.Summary{{Content: "昨日は途中で止まりがちだった"}},
		[]memory.Summary{{Content: "batch tool を増やした"}},
		[]memory.Fact{{Key: "autonomy-mode", Value: "前置きだけで止まらない"}},
	)

	if !strings.Contains(prompt, noReplyToken) {
		t.Fatalf("expected no-reply token, got %s", prompt)
	}
	if !strings.Contains(prompt, "Factorio") {
		t.Fatalf("expected presence activity, got %s", prompt)
	}
	if !strings.Contains(prompt, "少しでも価値があるなら自分から動いてよい") {
		t.Fatalf("expected proactive autonomy guidance, got %s", prompt)
	}
	if !strings.Contains(prompt, "best target channel") {
		t.Fatalf("expected target channel guidance, got %s", prompt)
	}
	if !strings.Contains(prompt, "open loops") || !strings.Contains(prompt, "agent-flow") {
		t.Fatalf("expected open loop context, got %s", prompt)
	}
	for _, want := range []string{"curiosities", "agent goals", "soft reminders", "topic threads", "initiatives", "behavior baselines", "behavior deviations"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected %q in autonomy prompt, got %s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "recent owner messages") {
		t.Fatalf("expected owner message context, got %s", prompt)
	}
	if !strings.Contains(prompt, "recent growth") || !strings.Contains(prompt, "recent decisions") {
		t.Fatalf("expected growth and decision context, got %s", prompt)
	}
}
