package antigravity

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCleanJSONSchemaForAntigravity(t *testing.T) {
	t.Run("nil schema gets placeholder reason", func(t *testing.T) {
		res := CleanJSONSchemaForAntigravity(nil)
		if res["type"] != "object" {
			t.Errorf("expected type object, got %v", res["type"])
		}
		props, ok := res["properties"].(map[string]any)
		if !ok || props["reason"] == nil {
			t.Errorf("expected placeholder reason property, got %#v", res)
		}
	})

	t.Run("strips unsupported keywords at all levels", func(t *testing.T) {
		schema := map[string]any{
			"$schema":              "http://json-schema.org/draft-07/schema#",
			"title":                "ToolInput",
			"additionalProperties": false,
			"type":                 "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":      "string",
					"minLength": 1,
					"maxLength": 100,
					"default":   "foo",
					"x-vendor":  "custom",
				},
				"options": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":        "string",
						"uniqueItems": true,
					},
				},
			},
			"required": []any{"path"},
		}

		cleaned := CleanJSONSchemaForAntigravity(schema)

		for _, bad := range []string{"$schema", "title", "additionalProperties"} {
			if cleaned[bad] != nil {
				t.Errorf("expected %q removed from root, got %v", bad, cleaned[bad])
			}
		}

		props := cleaned["properties"].(map[string]any)
		pathProp := props["path"].(map[string]any)
		for _, bad := range []string{"minLength", "maxLength", "default", "x-vendor"} {
			if pathProp[bad] != nil {
				t.Errorf("expected %q removed from path, got %v", bad, pathProp[bad])
			}
		}

		optsProp := props["options"].(map[string]any)
		itemsProp := optsProp["items"].(map[string]any)
		if itemsProp["uniqueItems"] != nil {
			t.Errorf("expected uniqueItems removed from items, got %v", itemsProp["uniqueItems"])
		}
	})

	t.Run("converts const to enum and ensures string enum values", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"const": "execute",
				},
				"code": map[string]any{
					"enum": []any{100, 200},
				},
			},
		}

		cleaned := CleanJSONSchemaForAntigravity(schema)
		props := cleaned["properties"].(map[string]any)

		actionProp := props["action"].(map[string]any)
		if actionProp["const"] != nil {
			t.Errorf("expected const removed")
		}
		if !reflect.DeepEqual(actionProp["enum"], []any{"execute"}) {
			t.Errorf("expected enum [execute], got %#v", actionProp["enum"])
		}

		codeProp := props["code"].(map[string]any)
		if codeProp["type"] != "string" {
			t.Errorf("expected enum type string, got %v", codeProp["type"])
		}
		if !reflect.DeepEqual(codeProp["enum"], []any{"100", "200"}) {
			t.Errorf("expected string enum values, got %#v", codeProp["enum"])
		}
	})

	t.Run("cleans invalid required properties", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"valid": map[string]any{"type": "string"},
			},
			"required": []string{"valid", "non_existent"},
		}

		cleaned := CleanJSONSchemaForAntigravity(schema)
		req, ok := cleaned["required"].([]any)
		if !ok || len(req) != 1 || req[0] != "valid" {
			t.Errorf("expected required to contain only ['valid'], got %#v", cleaned["required"])
		}
	})

	t.Run("flattens anyOf and oneOf selecting best schema", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"anyOf": []any{
						map[string]any{"type": "null"},
						map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		}

		cleaned := CleanJSONSchemaForAntigravity(schema)
		props := cleaned["properties"].(map[string]any)
		targetProp := props["target"].(map[string]any)

		if targetProp["anyOf"] != nil {
			t.Errorf("expected anyOf removed")
		}
		if targetProp["type"] != "object" {
			t.Errorf("expected best schema object selected, got %v", targetProp["type"])
		}
	})

	t.Run("serializes safely to JSON for Google", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to run",
				},
			},
			"required": []string{"command"},
		}

		cleaned := CleanJSONSchemaForAntigravity(schema)
		b, err := json.Marshal(cleaned)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		if !json.Valid(b) {
			t.Errorf("invalid json output")
		}
	})
}
