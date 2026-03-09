package voice

import (
	"strings"
	"testing"
)

func TestSessionInstructionsRequireJapanese(t *testing.T) {
	got := sessionInstructions("voice")
	for _, want := range []string{
		"名前は必ず ゆるり",
		"Kai など別名",
		"女性として",
		"一人称は必ず わたし",
		"1 文から 3 文",
		"ぶつ切り",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("session instructions missing %q\n%s", want, got)
		}
	}
}
