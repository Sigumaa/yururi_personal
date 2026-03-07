package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func TestParseAssistantReplyNoReply(t *testing.T) {
	got := parseAssistantReply(noReplyToken)
	if got.Action != assistantActionIgnore {
		t.Fatalf("expected ignore, got %s", got.Action)
	}
}

func TestParseAssistantReplyText(t *testing.T) {
	got := parseAssistantReply("こんにちは")
	if got.Action != assistantActionReply {
		t.Fatalf("expected reply, got %s", got.Action)
	}
	if got.Message != "こんにちは" {
		t.Fatalf("unexpected message: %s", got.Message)
	}
}

func TestSanitizeChannelName(t *testing.T) {
	got := sanitizeChannelName(" New Topic! 2026 ")
	if got != "new-topic-2026" {
		t.Fatalf("unexpected sanitized name: %s", got)
	}
}

func TestFallbackSummaryKeepsSoftTone(t *testing.T) {
	start := time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	summary := fallbackSummary("daily", start, end, []memory.Message{
		{ChannelName: "general", Content: "今日は調べ物をしてた"},
	})

	if got := "daily のまとめをそっと置いておきますね。"; !strings.HasPrefix(summary, got) {
		t.Fatalf("unexpected summary tone: %s", summary)
	}
}
