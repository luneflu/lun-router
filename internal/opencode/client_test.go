package opencode

import (
	"strings"
	"testing"
)

func TestSanitizeAssistantReasoning(t *testing.T) {
	tests := []struct {
		name         string
		messages     []any
		checkIndex   int
		wantRC       string
		wantContent  any
	}{
		{
			name: "assistant missing reasoning_content gets empty string",
			messages: []any{
				map[string]any{"role": "user", "content": "hi"},
				map[string]any{"role": "assistant", "content": "hello there"},
			},
			checkIndex:  1,
			wantRC:      "",
			wantContent: "hello there",
		},
		{
			name: "assistant nil reasoning_content gets empty string",
			messages: []any{
				map[string]any{"role": "assistant", "content": "hello", "reasoning_content": nil},
			},
			checkIndex:  0,
			wantRC:      "",
			wantContent: "hello",
		},
		{
			name: "assistant existing reasoning_content preserved",
			messages: []any{
				map[string]any{"role": "assistant", "content": "answer", "reasoning_content": "thinking step"},
			},
			checkIndex:  0,
			wantRC:      "thinking step",
			wantContent: "answer",
		},
		{
			name: "assistant think tags extracted into reasoning_content",
			messages: []any{
				map[string]any{
					"role":    "assistant",
					"content": "<think>\nstep 1\nstep 2\n</think>\nFinal answer.",
				},
			},
			checkIndex:  0,
			wantRC:      "step 1\nstep 2",
			wantContent: "Final answer.",
		},
		{
			name: "assistant multipart content with think tag",
			messages: []any{
				map[string]any{
					"role": "assistant",
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "<think>analyzing</think>Result",
						},
					},
				},
			},
			checkIndex: 0,
			wantRC:     "analyzing",
		},
		{
			name: "user message untouched",
			messages: []any{
				map[string]any{"role": "user", "content": "hi"},
			},
			checkIndex: 0,
			wantRC:     "__UNSET__",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyMap := map[string]any{
				"messages": tt.messages,
			}
			sanitizeAssistantReasoning(bodyMap)

			msgs := bodyMap["messages"].([]any)
			target := msgs[tt.checkIndex].(map[string]any)

			rc, hasRC := target["reasoning_content"]
			if tt.wantRC == "__UNSET__" {
				if hasRC {
					t.Errorf("expected no reasoning_content, got %v", rc)
				}
				return
			}

			if !hasRC {
				t.Fatalf("expected reasoning_content, got none")
			}
			if rc != tt.wantRC {
				t.Errorf("reasoning_content = %q, want %q", rc, tt.wantRC)
			}

			if tt.wantContent != nil {
				if target["content"] != tt.wantContent {
					t.Errorf("content = %q, want %q", target["content"], tt.wantContent)
				}
			}
		})
	}
}

func TestAggregateSSEStreamReasoningContent(t *testing.T) {
	sseData := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","reasoning_content":"Thinking..."}}]}`,
		`data: {"choices":[{"delta":{"content":"Answer!"}}]}`,
		`data: [DONE]`,
	}, "\n")

	resp, err := AggregateSSEStream(strings.NewReader(sseData), "test-model", nil)
	if err != nil {
		t.Fatalf("AggregateSSEStream failed: %v", err)
	}

	choices, ok := resp["choices"].([]map[string]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("invalid choices: %v", resp)
	}

	msg, ok := choices[0]["message"].(map[string]any)
	if !ok {
		t.Fatalf("invalid message: %v", choices[0])
	}

	if msg["content"] != "Answer!" {
		t.Errorf("expected content 'Answer!', got %q", msg["content"])
	}
	if msg["reasoning_content"] != "Thinking..." {
		t.Errorf("expected reasoning_content 'Thinking...', got %q", msg["reasoning_content"])
	}
}
