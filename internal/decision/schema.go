package decision

func OutputSchema() map[string]any {
	return strictObject(map[string]any{
		"action": schemaEnum(
			string(ActionIgnore),
			string(ActionReply),
			string(ActionSchedule),
			string(ActionAct),
			string(ActionReflect),
		),
		"reason":  schemaString(),
		"message": schemaString(),
		"confidence": map[string]any{
			"type": "number",
		},
		"memory_writes": map[string]any{
			"type": "array",
			"items": strictObject(map[string]any{
				"kind":  schemaString(),
				"key":   schemaString(),
				"value": schemaString(),
			}),
		},
		"jobs": map[string]any{
			"type": "array",
			"items": strictObject(map[string]any{
				"kind":        schemaString(),
				"title":       schemaString(),
				"channel_id":  schemaString(),
				"schedule":    schemaString(),
				"description": schemaString(),
				"payload": strictObject(map[string]any{
					"repo":       schemaString(),
					"since":      schemaString(),
					"url":        schemaString(),
					"query":      schemaString(),
					"prompt":     schemaString(),
					"goal":       schemaString(),
					"channel_id": schemaString(),
					"note":       schemaString(),
				}),
			}),
		},
		"actions": map[string]any{
			"type": "array",
			"items": strictObject(map[string]any{
				"type":              schemaString(),
				"name":              schemaString(),
				"parent_channel_id": schemaString(),
				"target_channel_id": schemaString(),
				"topic":             schemaString(),
				"reason":            schemaString(),
				"announcement_text": schemaString(),
			}),
		},
	})
}

func strictObject(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for key := range properties {
		required = append(required, key)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

func schemaString() map[string]any {
	return map[string]any{
		"type": "string",
	}
}

func schemaEnum(values ...string) map[string]any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return map[string]any{
		"type": "string",
		"enum": out,
	}
}
