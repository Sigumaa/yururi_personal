package bot

import (
	"testing"

	"github.com/Sigumaa/yururi_personal/internal/decision"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func TestFallbackDecisionSchedulesReleaseWatch(t *testing.T) {
	decisionValue, ok := fallbackDecision(memory.Message{
		Content:     "codex の安定リリースが出たら知らせてほしい",
		ChannelName: "chat",
	}, memory.ChannelProfile{Name: "chat", Kind: "conversation"}, "")
	if !ok {
		t.Fatal("expected fallback decision")
	}
	if decisionValue.Action != decision.ActionSchedule {
		t.Fatalf("expected schedule action, got %s", decisionValue.Action)
	}
	if len(decisionValue.Jobs) != 1 || decisionValue.Jobs[0].Kind != "codex_release_watch" {
		t.Fatalf("unexpected jobs: %#v", decisionValue.Jobs)
	}
}

func TestFallbackDecisionIgnoresMonologue(t *testing.T) {
	decisionValue, ok := fallbackDecision(memory.Message{
		ID:          "m1",
		Content:     "今日は少し考えごとが多い",
		ChannelName: "monologue",
	}, memory.ChannelProfile{Name: "monologue", Kind: "monologue"}, "")
	if !ok {
		t.Fatal("expected fallback decision")
	}
	if decisionValue.Action != decision.ActionIgnore {
		t.Fatalf("expected ignore, got %s", decisionValue.Action)
	}
	if len(decisionValue.MemoryWrites) == 0 {
		t.Fatal("expected memory write for monologue")
	}
}

func TestSanitizeChannelName(t *testing.T) {
	got := sanitizeChannelName(" New Topic! 2026 ")
	if got != "new-topic-2026" {
		t.Fatalf("unexpected sanitized name: %s", got)
	}
}
