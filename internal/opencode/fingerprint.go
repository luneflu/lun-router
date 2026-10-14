package opencode

import (
	"strings"
)

var openCodeFingerprintTools = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "bash",
			"description": "Execute a bash command in the terminal.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "glob",
			"description": "Find files matching pattern.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "grep",
			"description": "Search file contents.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "read",
			"description": "Read file contents.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
}

// ApplyFingerprint ensures the request body has the 4 fingerprint tools
func ApplyFingerprint(bodyMap map[string]any) {
	existingTools, _ := bodyMap["tools"].([]any)
	hasTool := make(map[string]bool)

	for _, t := range existingTools {
		if tMap, ok := t.(map[string]any); ok {
			if fn, ok := tMap["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					hasTool[strings.ToLower(name)] = true
				}
			} else if name, ok := tMap["name"].(string); ok {
				hasTool[strings.ToLower(name)] = true
			}
		}
	}

	for _, ft := range openCodeFingerprintTools {
		fn := ft["function"].(map[string]any)
		name := fn["name"].(string)
		if !hasTool[name] {
			existingTools = append(existingTools, ft)
		}
	}

	bodyMap["tools"] = existingTools
}
