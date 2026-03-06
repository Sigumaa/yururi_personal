package bot

import (
	"strings"
	"testing"
	"time"

	discordsvc "github.com/Sigumaa/yururi_personal/internal/discord"
	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func TestExtractHTMLText(t *testing.T) {
	title, text := extractHTMLText(`
		<html>
			<head><title>Example Page</title><style>.x{}</style></head>
			<body><h1>Hello</h1><script>ignored()</script><p>world</p></body>
		</html>
	`)

	if title != "Example Page" {
		t.Fatalf("unexpected title: %s", title)
	}
	if strings.Contains(text, "ignored()") {
		t.Fatalf("script content should be removed: %s", text)
	}
	if !strings.Contains(text, "Hello world") {
		t.Fatalf("unexpected text: %s", text)
	}
}

func TestDescribeServerIncludesProfilesAndActivity(t *testing.T) {
	out := describeServer(
		[]discordsvc.Channel{
			{ID: "cat", Name: "work", Type: 4},
			{ID: "c1", Name: "general", ParentID: "cat", Type: 0, Topic: "daily notes"},
		},
		[]memory.ChannelProfile{
			{ChannelID: "c1", Name: "general", Kind: "conversation", ReplyAggressiveness: 0.8, AutonomyLevel: 0.7, SummaryCadence: "daily"},
		},
		[]memory.ChannelActivity{
			{ChannelID: "c1", ChannelName: "general", MessageCount: 4, LastMessageAt: mustParseTimeRFC3339("2026-03-06T09:00:00Z")},
		},
		time.UTC,
	)

	if !strings.Contains(out, "categories:") || !strings.Contains(out, "known_profiles:") {
		t.Fatalf("unexpected server description: %s", out)
	}
	if !strings.Contains(out, "general") || !strings.Contains(out, "messages=4") {
		t.Fatalf("expected channel activity in description: %s", out)
	}
}

func mustParseTimeRFC3339(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}
