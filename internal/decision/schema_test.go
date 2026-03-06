package decision

import "testing"

func TestOutputSchemaIsStrict(t *testing.T) {
	schema := OutputSchema()
	if schema["additionalProperties"] != false {
		t.Fatalf("top-level schema must disable additionalProperties: %#v", schema["additionalProperties"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing")
	}
	for _, key := range []string{"memory_writes", "jobs", "actions"} {
		node, ok := props[key].(map[string]any)
		if !ok {
			t.Fatalf("property %s missing", key)
		}
		items, ok := node["items"].(map[string]any)
		if !ok {
			t.Fatalf("property %s items missing", key)
		}
		if items["additionalProperties"] != false {
			t.Fatalf("%s items must disable additionalProperties", key)
		}
	}

	jobsNode := props["jobs"].(map[string]any)
	jobItems := jobsNode["items"].(map[string]any)
	jobProps := jobItems["properties"].(map[string]any)
	payload := jobProps["payload"].(map[string]any)
	if payload["additionalProperties"] != false {
		t.Fatalf("jobs.payload must disable additionalProperties")
	}
}
