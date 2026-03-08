package voice

import "strings"

func previewText(value string, maxChars int) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxChars <= 0 || len(normalized) <= maxChars {
		return normalized
	}
	if maxChars <= 1 {
		return normalized[:maxChars]
	}
	return normalized[:maxChars-1] + "…"
}
