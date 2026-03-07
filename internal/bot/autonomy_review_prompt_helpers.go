package bot

import (
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/memory"
)

func formatFactLines(items []memory.Fact, truncateAt int) []string {
	if len(items) == 0 {
		return []string{"- none"}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		value := item.Value
		if truncateAt > 0 {
			value = truncateText(value, truncateAt)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", item.Key, value))
	}
	return lines
}

func formatMessageLines(items []memory.Message, truncateAt int) []string {
	if len(items) == 0 {
		return []string{"- none"}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- [%s/%s] %s", item.CreatedAt.Format("01-02 15:04"), item.ChannelName, truncateText(item.Content, truncateAt)))
	}
	return lines
}

func formatSummaryLines(items []memory.Summary, truncateAt int) []string {
	if len(items) == 0 {
		return []string{"- none"}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- %s", truncateText(item.Content, truncateAt)))
	}
	return lines
}

func joinPromptLines(items []string) string {
	return strings.Join(items, "\n")
}
