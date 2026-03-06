package decision

func OutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"action", "reason"},
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					string(ActionIgnore),
					string(ActionReply),
					string(ActionSchedule),
					string(ActionAct),
					string(ActionReflect),
				},
			},
			"reason": map[string]any{
				"type": "string",
			},
			"message": map[string]any{
				"type": "string",
			},
			"confidence": map[string]any{
				"type": "number",
			},
			"memory_writes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"kind", "key", "value"},
					"properties": map[string]any{
						"kind":  map[string]any{"type": "string"},
						"key":   map[string]any{"type": "string"},
						"value": map[string]any{"type": "string"},
					},
				},
			},
			"jobs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"kind", "title"},
					"properties": map[string]any{
						"kind":        map[string]any{"type": "string"},
						"title":       map[string]any{"type": "string"},
						"channel_id":  map[string]any{"type": "string"},
						"schedule":    map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"payload": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]any{
								"repo":       map[string]any{"type": "string"},
								"since":      map[string]any{"type": "string"},
								"url":        map[string]any{"type": "string"},
								"query":      map[string]any{"type": "string"},
								"channel_id": map[string]any{"type": "string"},
								"note":       map[string]any{"type": "string"},
							},
						},
					},
				},
			},
			"actions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"type"},
					"properties": map[string]any{
						"type":              map[string]any{"type": "string"},
						"name":              map[string]any{"type": "string"},
						"parent_channel_id": map[string]any{"type": "string"},
						"target_channel_id": map[string]any{"type": "string"},
						"topic":             map[string]any{"type": "string"},
						"reason":            map[string]any{"type": "string"},
						"announcement_text": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}
