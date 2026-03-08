package voice

import "testing"

func TestDownsampleDiscordToRealtime(t *testing.T) {
	input := []int16{
		100, -100,
		300, -300,
		500, -500,
		700, -700,
	}
	got := downsampleDiscordToRealtime(input)
	want := []int16{0, 0}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected sample at %d: got=%d want=%d", i, got[i], want[i])
		}
	}
}

func TestUpsampleRealtimeToDiscord(t *testing.T) {
	got := upsampleRealtimeToDiscord([]int16{10, 20})
	want := []int16{10, 10, 15, 15, 20, 20, 20, 20}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected sample at %d: got=%d want=%d", i, got[i], want[i])
		}
	}
}
