package discord

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestActivitiesFromGatewayCapturesRichPresence(t *testing.T) {
	start := time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC)
	end := start.Add(4 * time.Minute)
	items := ActivitiesFromGateway([]*discordgo.Activity{
		{
			Name:      "Spotify",
			Type:      discordgo.ActivityTypeListening,
			Details:   "Blue Train",
			State:     "John Coltrane",
			CreatedAt: start,
			Timestamps: discordgo.TimeStamps{
				StartTimestamp: start.UnixMilli(),
				EndTimestamp:   end.UnixMilli(),
			},
			Assets: discordgo.Assets{
				LargeText: "Blue Train",
				SmallText: "Spotify",
			},
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 activity, got %#v", items)
	}
	if items[0].Type != "listening" || items[0].Details != "Blue Train" || items[0].State != "John Coltrane" {
		t.Fatalf("unexpected mapped activity: %#v", items[0])
	}
	if items[0].StartAt == nil || items[0].EndAt == nil {
		t.Fatalf("expected timestamps, got %#v", items[0])
	}
}

func TestActivityTypeNameHasReadableLabels(t *testing.T) {
	for _, tc := range []struct {
		in   discordgo.ActivityType
		want string
	}{
		{discordgo.ActivityTypeGame, "playing"},
		{discordgo.ActivityTypeStreaming, "streaming"},
		{discordgo.ActivityTypeListening, "listening"},
		{discordgo.ActivityTypeWatching, "watching"},
		{discordgo.ActivityTypeCustom, "custom"},
		{discordgo.ActivityTypeCompeting, "competing"},
		{discordgo.ActivityType(99), "type_99"},
	} {
		if got := activityTypeName(tc.in); got != tc.want {
			t.Fatalf("expected %s, got %s", tc.want, got)
		}
	}
	if got := activityTypeName(discordgo.ActivityTypeListening); strings.TrimSpace(got) == "" {
		t.Fatalf("expected non-empty type label, got %q", got)
	}
}
