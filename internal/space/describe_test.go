package space

import (
	"strings"
	"testing"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
	"github.com/bwmarrin/discordgo"
)

func TestDescribeServerIncludesProfilesAndActivity(t *testing.T) {
	t.Parallel()

	out := DescribeServer(
		[]discordsvc.Channel{
			{ID: "cat", Name: "lab", Type: discordgo.ChannelTypeGuildCategory},
			{ID: "c1", Name: "general", ParentID: "cat", Type: discordgo.ChannelTypeGuildText, Topic: "main"},
		},
		[]memory.ChannelProfile{
			{ChannelID: "c1", Name: "general", Kind: "conversation", ReplyAggressiveness: 0.8, AutonomyLevel: 0.6, SummaryCadence: "daily"},
		},
		[]memory.ChannelActivity{
			{ChannelID: "c1", MessageCount: 4, LastMessageAt: time.Date(2026, 3, 7, 3, 4, 5, 0, time.UTC)},
		},
		time.UTC,
	)

	for _, want := range []string{"categories:", "lab", "general", "profile=conversation", "messages=4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestDiffRecentSnapshotsShowsAddedAndRemoved(t *testing.T) {
	t.Parallel()

	diff := DiffRecentSnapshots(
		"snapshot label: old\nsince_hours: 10\n- a\n- b",
		"snapshot label: new\nsince_hours: 10\n- b\n- c",
	)

	for _, want := range []string{"newer: snapshot label: new", "older: snapshot label: old", "added:", "+ - c", "removed:", "- - a"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("missing %q in diff: %s", want, diff)
		}
	}
}
