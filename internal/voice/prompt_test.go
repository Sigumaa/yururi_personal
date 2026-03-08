package voice

import (
	"strings"
	"testing"
)

func TestSessionInstructionsRequireJapanese(t *testing.T) {
	got := sessionInstructions("voice")
	for _, want := range []string{
		"あなたは女性として話します。",
		"一人称は自然な範囲でも必ず わたし を使い",
		"返答は必ず自然な日本語で行ってください。",
		"英語や他言語へ勝手に切り替えないでください。",
		"音声の返答は基本 1 文から 3 文でまとめ",
		"語の途中で不自然に間を空けたり",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("session instructions missing %q\n%s", want, got)
		}
	}
}
