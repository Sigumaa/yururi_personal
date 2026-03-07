package bot

import (
	"testing"
	"time"
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

func TestSummarizeMessagesRequiresThread(t *testing.T) {
	app := &App{}
	if _, err := app.summarizeMessages(t.Context(), "", "daily", time.Time{}, time.Time{}, nil); err == nil {
		t.Fatal("expected missing thread error")
	}
}
