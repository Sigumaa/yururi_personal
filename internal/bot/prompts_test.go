package bot

import (
	"strings"
	"testing"
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
}
