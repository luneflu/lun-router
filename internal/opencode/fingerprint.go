package opencode

import (
	"strings"
)

// Verbatim definitions from OpenCode v2 CLI.
// Required by upstream /inference/v1 free-tier gate.
var (
	readDescription = "Read the contents of a file or directory. Supports text files, images, and PDFs. Images and PDFs are presented directly to the model. Each text line is prefixed by its 1-based line number as <line>: <content>. The prefix is for reference and is not part of the file content. Directory entries are returned one per line. Use offset and limit to read large files or directories in sections. Prefer one larger read over many small slices, and use grep to find specific content in large files."
	readParameters  = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "File or directory to read"},
			"offset": map[string]any{"type": "integer", "minimum": 0, "description": "The line or directory entry to start reading from (1-based)"},
			"limit":  map[string]any{"type": "integer", "minimum": 0, "description": "The maximum number of lines or directory entries to read (defaults to and capped at 2000)"},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}

	shellDescription = "Execute a shell command and return its output. Commands run on Linux using bash. Quote file paths containing spaces or special characters. Prefer dedicated tools over shell commands when possible. When output is large, the full result is saved to a file and a truncated preview is returned. Rely on automatic truncation unless filtering the output is more useful. Commands accept an optional timeout, background commands have no timeout by default. Background commands return immediately, and you will be notified when they complete."
	shellParameters  = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string", "description": "Shell command string to execute"},
			"workdir":    map[string]any{"type": "string", "description": "Working directory to execute the command in. Defaults to the current working directory. When possible, avoid changing directories in the command and set the working directory here instead."},
			"timeout":    map[string]any{"type": "integer", "minimum": 0, "description": "Timeout in milliseconds. Set to 0 to disable the timeout. Defaults to 120000 for foreground commands. Background commands have no timeout by default."},
			"background": map[string]any{"type": "boolean", "description": "Run the command in the background and return immediately (useful for dev servers and long-running builds). You do not need to use '&' at the end of the command when using this parameter. You will be notified when it completes. DO NOT poll for completion."},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	}

	// Nested shape for OpenAI Chat Completions endpoint
	ChatFingerprintTools = []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "read",
				"description": readDescription,
				"parameters":  readParameters,
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        "shell",
				"description": shellDescription,
				"parameters":  shellParameters,
			},
		},
	}

	// Flat shape for OpenAI Responses endpoint
	ResponsesFingerprintTools = []map[string]any{
		{
			"type":        "function",
			"name":        "read",
			"description": readDescription,
			"parameters":  readParameters,
			"strict":      false,
		},
		{
			"type":        "function",
			"name":        "shell",
			"description": shellDescription,
			"parameters":  shellParameters,
			"strict":      false,
		},
	}
)

// ApplyFingerprint ensures the request body has read and shell fingerprint tools
func ApplyFingerprint(bodyMap map[string]any, isResponses bool) {
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

	toolDefs := ChatFingerprintTools
	if isResponses {
		toolDefs = ResponsesFingerprintTools
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
