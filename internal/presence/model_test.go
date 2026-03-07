package presence

import (
	"strings"
	"testing"
	"time"
)

func TestSummaryAndDescribeList(t *testing.T) {
	start := time.Date(2026, 3, 8, 1, 2, 3, 0, time.UTC)
	end := start.Add(3 * time.Minute)
	items := []Activity{
		{
			Name:      "Spotify",
			Type:      "listening",
			Details:   "Blue Train",
			State:     "John Coltrane",
			LargeText: "Blue Train",
			StartAt:   &start,
			EndAt:     &end,
		},
	}

	if got := Summary(items[0]); !strings.Contains(got, "Blue Train") || !strings.Contains(got, "John Coltrane") {
		t.Fatalf("unexpected summary: %s", got)
	}

	raw := DescribeList(items)
	for _, want := range []string{"type=listening", "name=Spotify", "details=Blue Train", "state=John Coltrane", "large_text=Blue Train", "start_at=2026-03-08T01:02:03Z"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in described activities, got %s", want, raw)
		}
	}
}
