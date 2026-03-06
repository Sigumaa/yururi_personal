package codex

import (
	"encoding/json"
	"strings"
)

func previewText(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	return strings.TrimSpace(value[:maxChars]) + "..."
}

func previewJSON(value any, maxChars int) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "json_marshal_error"
	}
	return previewText(string(raw), maxChars)
}

func previewToolResponse(response ToolResponse, maxChars int) string {
	return previewJSON(response, maxChars)
}
