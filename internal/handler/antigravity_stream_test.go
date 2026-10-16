package handler

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"lun-router/internal/usage"
)

func TestStreamAntigravityAsOpenAIWithToolCallsAndReasoning(t *testing.T) {
	// Simulate an Antigravity SSE stream containing thought and functionCall
	sseData := `data: {"response":{"candidates":[{"content":{"parts":[{"text":"Thinking about files...","thought":true}]}}]}}

data: {"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"read","args":{"path":"src/app.html"}}}]},"finishReason":"STOP"}]}}

data: [DONE]

`
	var out bytes.Buffer
	tk := &usage.Tokens{}
	streamAntigravityAsOpenAI(&out, nil, false, strings.NewReader(sseData), "gemini-3.8-flash", tk)

	lines := strings.Split(out.String(), "\n")
	var hasRole bool
	var hasReasoning bool
	var hasToolCall bool
	var finalFinishReason string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			continue
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)

		if r, _ := delta["role"].(string); r == "assistant" {
			hasRole = true
		}
		if reasoning, _ := delta["reasoning_content"].(string); reasoning == "Thinking about files..." {
			hasReasoning = true
		}
		if tcs, ok := delta["tool_calls"].([]any); ok && len(tcs) > 0 {
			tc := tcs[0].(map[string]any)
			fn := tc["function"].(map[string]any)
			if fn["name"] == "read" && strings.Contains(fn["arguments"].(string), "src/app.html") {
				hasToolCall = true
			}
		}
		if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
			finalFinishReason = fr
		}
	}

	if !hasRole {
		t.Errorf("expected role chunk")
	}
	if !hasReasoning {
		t.Errorf("expected delta with reasoning_content")
	}
	if !hasToolCall {
		t.Errorf("expected delta with tool_calls")
	}
	if finalFinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %q", finalFinishReason)
	}
}
