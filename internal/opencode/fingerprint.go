package opencode

import (
	"strings"
)

var (
	unavailableDesc = "This tool is currently unavailable and must not be used."
	emptyParams     = map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}

	// Canonical tools required by OpenCode Zen free-tier gate: bash, shell, glob, grep, read
	ChatQuartetTools = []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "bash",
				"description": unavailableDesc,
				"parameters":  emptyParams,
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "shell",
				"description": unavailableDesc,
				"parameters":  emptyParams,
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "glob",
				"description": unavailableDesc,
				"parameters":  emptyParams,
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "grep",
				"description": unavailableDesc,
				"parameters":  emptyParams,
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "read",
				"description": unavailableDesc,
				"parameters":  emptyParams,
			},
		},
	}

	ResponsesQuartetTools = []map[string]any{
		{
			"type":        "function",
			"name":        "bash",
			"description": unavailableDesc,
			"parameters":  emptyParams,
		},
		{
			"type":        "function",
			"name":        "shell",
			"description": unavailableDesc,
			"parameters":  emptyParams,
		},
		{
			"type":        "function",
			"name":        "glob",
			"description": unavailableDesc,
			"parameters":  emptyParams,
		},
		{
			"type":        "function",
			"name":        "grep",
			"description": unavailableDesc,
			"parameters":  emptyParams,
		},
		{
			"type":        "function",
			"name":        "read",
			"description": unavailableDesc,
			"parameters":  emptyParams,
		},
	}
)

// ApplyFingerprint ensures the request body has the complete quartet (bash, glob, grep, read)
func ApplyFingerprint(bodyMap map[string]any, isResponses bool) {
	existingTools, _ := bodyMap["tools"].([]any)
	hasTool := make(map[string]bool)

	for _, t := range existingTools {
		if tMap, ok := t.(map[string]any); ok {
			if fn, ok := tMap["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					hasTool[strings.ToLower(strings.TrimSpace(name))] = true
				}
			} else if name, ok := tMap["name"].(string); ok {
				hasTool[strings.ToLower(strings.TrimSpace(name))] = true
			}
		}
	}

	toolDefs := ChatQuartetTools
	if isResponses {
		toolDefs = ResponsesQuartetTools
	}

	for _, ft := range toolDefs {
		var name string
		if isResponses {
			name = ft["name"].(string)
		} else {
			fn := ft["function"].(map[string]any)
			name = fn["name"].(string)
		}
		if !hasTool[name] {
			existingTools = append(existingTools, ft)
		}
	}

	bodyMap["tools"] = existingTools

	// Never send tool_choice: "none" (upstream rejects with 403 FreeTierError)
	if tc, ok := bodyMap["tool_choice"].(string); ok && tc == "none" {
		delete(bodyMap, "tool_choice")
	}
	if isResponses && bodyMap["tool_choice"] == nil {
		bodyMap["tool_choice"] = "auto"
	}
}
