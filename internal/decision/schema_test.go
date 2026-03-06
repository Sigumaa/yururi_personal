package decision

import "testing"

func TestOutputSchemaIsStrict(t *testing.T) {
	assertStrictObjectSchema(t, "root", OutputSchema())
}

func assertStrictObjectSchema(t *testing.T, path string, node map[string]any) {
	t.Helper()

	properties, hasProperties := node["properties"].(map[string]any)
	if hasProperties {
		if node["type"] != "object" {
			t.Fatalf("%s must be object: %#v", path, node["type"])
		}
		if node["additionalProperties"] != false {
			t.Fatalf("%s must disable additionalProperties: %#v", path, node["additionalProperties"])
		}

		required, ok := node["required"].([]string)
		if !ok {
			t.Fatalf("%s must declare required fields", path)
		}
		if len(required) != len(properties) {
			t.Fatalf("%s required count mismatch: got=%d want=%d", path, len(required), len(properties))
		}

		requiredSet := map[string]bool{}
		for _, key := range required {
			requiredSet[key] = true
		}
		for key, rawChild := range properties {
			if !requiredSet[key] {
				t.Fatalf("%s missing required key %q", path, key)
			}
			child, ok := rawChild.(map[string]any)
			if !ok {
				continue
			}
			assertStrictChildSchemas(t, path+"."+key, child)
		}
		return
	}

	assertStrictChildSchemas(t, path, node)
}

func assertStrictChildSchemas(t *testing.T, path string, node map[string]any) {
	t.Helper()

	if items, ok := node["items"].(map[string]any); ok {
		assertStrictObjectSchema(t, path+"[]", items)
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		assertStrictObjectSchema(t, path, map[string]any{
			"type":                 "object",
			"additionalProperties": node["additionalProperties"],
			"required":             node["required"],
			"properties":           properties,
		})
	}
}
