package commandcode

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"
)

// defaultMaxTokens mirrors the CLI's xT default in /alpha/generate params.
const defaultMaxTokens = 64000

// BuildBody converts an OpenAI chat.completions body into the /alpha/generate
// envelope the Command Code CLI sends (verified against command-code@1.84.0):
//
//	{config, memory, taste, skills, permissionMode, threadId, params{...}}
//
// params.messages use Anthropic-style content blocks, and system messages live in
// params.system instead of messages[].
func BuildBody(body map[string]any, workingDir string) (map[string]any, error) {
	rawMessages := asList(body["messages"])

	var messages []any
	var systemTexts []string
	toolNames := map[string]string{}

	for _, raw := range rawMessages {
		m, ok := asMap(raw)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)

		switch role {
		case "system", "developer":
			if text := flattenText(m["content"]); text != "" {
				systemTexts = append(systemTexts, text)
			}

		case "assistant":
			blocks := []any{}
			for _, key := range []string{"reasoning_content", "thinking", "reasoning"} {
				if text, ok := m[key].(string); ok && text != "" {
					blocks = append(blocks, map[string]any{"type": "reasoning", "text": text})
					break
				}
			}
			if text := flattenText(m["content"]); text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": text})
			}
			if calls := asList(m["tool_calls"]); len(calls) > 0 {
				for _, rawCall := range calls {
					call, ok := asMap(rawCall)
					if !ok {
						continue
					}
					id, _ := call["id"].(string)
					fn, _ := call["function"].(map[string]any)
					name, _ := fn["name"].(string)
					if name == "" {
						continue
					}
					toolNames[id] = name
					blocks = append(blocks, map[string]any{
						"type":       "tool-call",
						"toolCallId": id,
						"toolName":   name,
						"input":      parseArguments(fn["arguments"]),
					})
				}
			}
			if len(blocks) == 0 {
				blocks = append(blocks, map[string]any{"type": "text", "text": ""})
			}
			messages = append(messages, map[string]any{"role": "assistant", "content": blocks})

		case "tool":
			id, _ := m["tool_call_id"].(string)
			name, _ := m["name"].(string)
			if name == "" {
				name = toolNames[id]
			}
			if name == "" {
				name = "unknown"
			}
			messages = append(messages, map[string]any{
				"role": "tool",
				"content": []any{map[string]any{
					"type":       "tool-result",
					"toolCallId": id,
					"toolName":   name,
					"output":     map[string]any{"type": "text", "value": flattenText(m["content"])},
				}},
			})

		default:
			messages = append(messages, map[string]any{"role": "user", "content": userBlocks(m["content"])})
		}
	}

	model, _ := body["model"].(string)
	if messages == nil {
		messages = []any{}
	}
	params := map[string]any{
		"model":      CleanModelID(model),
		"messages":   messages,
		"stream":     true,
		"max_tokens": maxTokens(body),
	}
	if len(systemTexts) > 0 {
		params["system"] = []any{map[string]any{"type": "text", "text": strings.Join(systemTexts, "\n\n")}}
	}
	if tools := wireTools(body["tools"]); len(tools) > 0 {
		params["tools"] = tools
	}
	if temperature, ok := body["temperature"].(float64); ok {
		params["temperature"] = temperature
	}
	if effort, ok := body["reasoning_effort"].(string); ok && effort != "" {
		params["reasoning_effort"] = effort
	}

	return map[string]any{
		"config":         serverConfig(workingDir),
		"memory":         nil,
		"taste":          nil,
		"skills":         nil,
		"permissionMode": "standard",
		"threadId":       newUUID(),
		"params":         params,
	}, nil
}

func serverConfig(workingDir string) map[string]any {
	return map[string]any{
		"workingDir":    workingDir,
		"date":          time.Now().UTC().Format("2006-01-02"),
		"environment":   runtime.GOOS,
		"structure":     []any{},
		"isGitRepo":     false,
		"currentBranch": "",
		"mainBranch":    "",
		"gitStatus":     "",
		"recentCommits": []any{},
	}
}

func maxTokens(body map[string]any) int {
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if n, ok := body[key].(float64); ok && n > 0 {
			return int(n)
		}
	}
	return defaultMaxTokens
}

func wireTools(raw any) []any {
	list := asList(raw)
	tools := make([]any, 0, len(list))
	for _, item := range list {
		t, ok := asMap(item)
		if !ok {
			continue
		}
		fn, ok := asMap(t["function"])
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}
		schema := fn["parameters"]
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		tools = append(tools, map[string]any{
			"name":         name,
			"description":  fn["description"],
			"input_schema": schema,
		})
	}
	return tools
}

func userBlocks(content any) []any {
	var blocks []any

	addText := func(text string) {
		if text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
	}
	addImage := func(url string) {
		mime := dataURIMime(url)
		if mime == "" {
			return
		}
		blocks = append(blocks, map[string]any{"type": "image", "image": url, "mimeType": mime})
	}

	switch c := content.(type) {
	case string:
		addText(c)
	default:
		for _, raw := range asList(content) {
			part, ok := asMap(raw)
			if !ok {
				if text, ok := raw.(string); ok {
					addText(text)
				}
				continue
			}
			switch part["type"] {
			case "text", "input_text":
				if text, ok := part["text"].(string); ok {
					addText(text)
				}
			case "image_url":
				switch v := part["image_url"].(type) {
				case string:
					addImage(v)
				case map[string]any:
					if url, ok := v["url"].(string); ok {
						addImage(url)
					}
				}
			case "image":
				if data, ok := part["image"].(string); ok {
					addImage(data)
				}
			}
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": ""})
	}
	return blocks
}

func flattenText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		parts := make([]string, 0, len(c))
		for _, raw := range c {
			switch part := raw.(type) {
			case string:
				parts = append(parts, part)
			case map[string]any:
				if text, ok := part["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// dataURIMime returns the media type of a data: URI, or "" when the URL is not
// a data URI (upstream expects inline base64 images only).
func dataURIMime(url string) string {
	if !strings.HasPrefix(url, "data:") {
		return ""
	}
	rest := strings.TrimPrefix(url, "data:")
	semi := strings.Index(rest, ";")
	if semi <= 0 {
		return ""
	}
	return rest[:semi]
}

// asList normalizes any slice (JSON-decoded []any, or a Go literal such as
// []map[string]string) into []any; typed slices otherwise reach the wire as null.
func asList(raw any) []any {
	switch v := raw.(type) {
	case nil:
		return nil
	case []any:
		return v
	}
	rv := reflect.ValueOf(raw)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out = append(out, rv.Index(i).Interface())
	}
	return out
}

// asMap normalizes any string-keyed map (JSON-decoded map[string]any, or a Go
// literal such as map[string]string) into map[string]any.
func asMap(raw any) (map[string]any, bool) {
	if m, ok := raw.(map[string]any); ok {
		return m, true
	}
	rv := reflect.ValueOf(raw)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	out := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		out[iter.Key().String()] = iter.Value().Interface()
	}
	return out, true
}

func parseArguments(raw any) any {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return map[string]any{}
		}
		var out any
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return map[string]any{}
		}
		return out
	case nil:
		return map[string]any{}
	default:
		return v
	}
}

// MarshalBody is a small helper so callers can log the exact wire payload.
func MarshalBody(body map[string]any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal commandcode body: %w", err)
	}
	return data, nil
}
