package bot

import (
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/decision"
)

func TestParseAssistantReplyNoReply(t *testing.T) {
	got := parseAssistantReply(noReplyToken)
	if got.Action != decision.ActionIgnore {
		t.Fatalf("expected ignore, got %s", got.Action)
	}
}

func TestParseAssistantReplyText(t *testing.T) {
	got := parseAssistantReply("こんにちは")
	if got.Action != decision.ActionReply {
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
