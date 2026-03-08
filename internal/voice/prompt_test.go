package voice

import (
	"strings"
	"testing"
)

func TestSessionInstructionsRequireJapanese(t *testing.T) {
	got := sessionInstructions("voice")
	for _, want := range []string{
		"返答は必ず自然な日本語で行ってください。",
		"英語や他言語へ勝手に切り替えないでください。",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("session instructions missing %q\n%s", want, got)
		}
	}
}
