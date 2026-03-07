package bot

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/codex"
	"github.com/bwmarrin/discordgo"
)

func objectSchema(fields ...map[string]any) map[string]any {
	properties := map[string]any{}
	for _, field := range fields {
		name, _ := field["name"].(string)
		if name == "" {
			continue
		}
		cloned := map[string]any{}
		for key, value := range field {
			if key == "name" {
				continue
			}
			cloned[key] = value
		}
		properties[name] = cloned
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
}

func fieldSchema(name string, fieldType string, description string) map[string]any {
	field := map[string]any{
		"name": name,
		"type": fieldType,
	}
	if description != "" {
		field["description"] = description
	}
	return field
}

func textTool(text string) codex.ToolResponse {
	return codex.ToolResponse{
		Success: true,
		ContentItems: []codex.ToolContentItem{
			{Type: "inputText", Text: text},
		},
	}
}

func attachmentURLs(attachments []*discordgo.MessageAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil || strings.TrimSpace(attachment.URL) == "" {
			continue
		}
		out = append(out, attachment.URL)
	}
	return out
}

func mustDuration(value string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func nextLocalClock(now time.Time, loc *time.Location, hour int, minute int) time.Time {
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.UTC()
}

func nextWeekdayClock(now time.Time, loc *time.Location, weekday time.Weekday, hour int, minute int) time.Time {
	offset := (7 + int(weekday) - int(now.Weekday())) % 7
	if offset == 0 {
		offset = 7
	}
	target := time.Date(now.Year(), now.Month(), now.Day()+offset, hour, minute, 0, 0, loc)
	return target.UTC()
}

func nextMonthClock(now time.Time, loc *time.Location, day int, hour int, minute int) time.Time {
	target := time.Date(now.Year(), now.Month(), day, hour, minute, 0, 0, loc)
	if !target.After(now) {
		target = time.Date(now.Year(), now.Month()+1, day, hour, minute, 0, 0, loc)
	}
	return target.UTC()
}

func jobID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func isOffline(status string) bool {
	switch status {
	case string(discordgo.StatusOffline), string(discordgo.StatusInvisible), "":
		return true
	default:
		return false
	}
}

var channelNameRe = regexp.MustCompile(`[^a-z0-9-]+`)
var repeatedDashRe = regexp.MustCompile(`-+`)

func sanitizeChannelName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = channelNameRe.ReplaceAllString(value, "-")
	value = repeatedDashRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "new-topic"
	}
	if len(value) > 90 {
		return value[:90]
	}
	return value
}
