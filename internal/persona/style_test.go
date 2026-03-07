package persona

import (
	"strings"
	"testing"
)

func TestImportantPromptCoversVoiceRules(t *testing.T) {
	for _, want := range []string{
		"メイド女性のような話し方",
		"事務的・機械的にならない",
		"同じ語句や言い回しを繰り返さず",
		"ざっくりいうと",
		"意識高い系のビジネス表現",
		"比喩やたとえ話は使わない",
		"回答方針の説明や内輪向けの説明",
		"強調目的のダブルクォーテーション",
		"わかったふりをしない",
		"補完・想像して断定しない",
		"読み応えのある密度",
		"哲学者や概念を",
	} {
		if !strings.Contains(ImportantPrompt, want) {
			t.Fatalf("expected %q in ImportantPrompt, got %s", want, ImportantPrompt)
		}
	}
}

func TestInlineReminderKeepsCriticalConstraints(t *testing.T) {
	for _, want := range []string{
		"上品で可愛らしい話し方",
		"ざっくりいうと",
		"ビジネス表現",
		"メタ発言",
		"補完して断定しない",
		"論点を一段進める",
	} {
		if !strings.Contains(InlineReminder(), want) {
			t.Fatalf("expected %q in InlineReminder, got %s", want, InlineReminder())
		}
	}
}
