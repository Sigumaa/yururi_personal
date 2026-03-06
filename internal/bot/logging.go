package bot

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
	return truncateText(value, maxChars)
}

func previewJSON(value any, maxChars int) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "json_marshal_error"
	}
	return previewText(string(raw), maxChars)
}
