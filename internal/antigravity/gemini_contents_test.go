package antigravity

import (
	"reflect"
	"testing"
)

func TestNormalizeGeminiContentsTerminalTurnGuards(t *testing.T) {
	// 1. Appends user Continue turn when ending with model text turn
	t.Run("appends user Continue turn when ending with model text turn", func(t *testing.T) {
		contents := []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "hi"}}},
			{"role": "model", "parts": []map[string]any{{"text": "hello"}}},
		}
		out := normalizeGeminiContents(contents)
		if len(out) != 3 {
			t.Fatalf("expected 3 items, got %d", len(out))
		}
		if out[2]["role"] != "user" {
			t.Errorf("expected role user, got %v", out[2]["role"])
		}
		parts := out[2]["parts"].([]map[string]any)
		if parts[0]["text"] != "Continue." {
			t.Errorf("expected Continue., got %v", parts[0]["text"])
		}
	})

	// 2. Appends functionResponse user turn when ending with functionCall
	t.Run("appends functionResponse user turn when ending with functionCall", func(t *testing.T) {
		contents := []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "run"}}},
			{
				"role": "model",
				"parts": []map[string]any{
					{
						"functionCall": map[string]any{
							"id":   "call_1",
							"name": "search",
							"args": map[string]any{"q": "test"},
						},
					},
				},
			},
		}
		out := normalizeGeminiContents(contents)
		if len(out) != 3 {
			t.Fatalf("expected 3 items, got %d", len(out))
		}
		parts := out[2]["parts"].([]map[string]any)
		fr, ok := parts[0]["functionResponse"].(map[string]any)
		if !ok {
			t.Fatalf("expected functionResponse, got %#v", parts[0])
		}
		if fr["id"] != "call_1" || fr["name"] != "search" {
			t.Errorf("unexpected functionResponse fields: %#v", fr)
		}
	})

	// 3. Handles multiple functionCalls in terminal model turn
	t.Run("handles multiple functionCalls in terminal model turn", func(t *testing.T) {
		contents := []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "run"}}},
			{
				"role": "model",
				"parts": []map[string]any{
					{"functionCall": map[string]any{"id": "call_1", "name": "fn_1"}},
					{"functionCall": map[string]any{"id": "call_2", "name": "fn_2"}},
				},
			},
		}
		out := normalizeGeminiContents(contents)
		if len(out) != 3 {
			t.Fatalf("expected 3 items, got %d", len(out))
		}
		parts := out[2]["parts"].([]map[string]any)
		if len(parts) != 2 {
			t.Fatalf("expected 2 functionResponses, got %d", len(parts))
		}
	})

	// 4. Handles single model turn by prepending user prompt and appending terminal user
	t.Run("handles single model turn by prepending user prompt and appending terminal user", func(t *testing.T) {
		contents := []map[string]any{
			{"role": "model", "parts": []map[string]any{{"text": "prefill"}}},
		}
		out := normalizeGeminiContents(contents)
		if len(out) != 3 {
			t.Fatalf("expected 3 items, got %d", len(out))
		}
		if out[0]["role"] != "user" {
			t.Errorf("expected initial user turn, got %v", out[0]["role"])
		}
		if out[1]["role"] != "model" {
			t.Errorf("expected model turn in middle, got %v", out[1]["role"])
		}
		if out[2]["role"] != "user" {
			t.Errorf("expected terminal user turn, got %v", out[2]["role"])
		}
	})

	// 5. Does not mutate payloads already ending with a user turn
	t.Run("does not mutate payloads already ending with a user turn", func(t *testing.T) {
		contents := []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "question"}}},
		}
		out := normalizeGeminiContents(contents)
		if len(out) != 1 {
			t.Fatalf("expected 1 item, got %d", len(out))
		}
		if out[0]["role"] != "user" {
			t.Errorf("expected role user, got %v", out[0]["role"])
		}
	})

	// 6. Merges adjacent model turns before appending terminal user turn
	t.Run("merges adjacent model turns before appending terminal user turn", func(t *testing.T) {
		contents := []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "Prompt"}}},
			{"role": "model", "parts": []map[string]any{{"text": "Part A"}}},
			{"role": "model", "parts": []map[string]any{{"text": "Part B"}}},
		}
		out := normalizeGeminiContents(contents)
		if len(out) != 3 {
			t.Fatalf("expected 3 items, got %d", len(out))
		}
		if out[1]["role"] != "model" {
			t.Errorf("expected middle turn to be model, got %v", out[1]["role"])
		}
		parts := out[1]["parts"].([]map[string]any)
		if len(parts) != 2 {
			t.Errorf("expected 2 parts merged, got %d", len(parts))
		}
		if out[2]["role"] != "user" {
			t.Errorf("expected terminal user turn, got %v", out[2]["role"])
		}
	})

	// 7. Handles empty and nil inputs gracefully
	t.Run("handles empty input gracefully", func(t *testing.T) {
		out := normalizeGeminiContents(nil)
		if len(out) != 1 || out[0]["role"] != "user" {
			t.Errorf("expected fallback user turn, got %#v", out)
		}
	})
}

func TestConvertOpenAIMessagesToGeminiMultiTurn(t *testing.T) {
	// Exact user scenario:
	// Turn 1: user "Hi"
	// Turn 2: assistant "Ready. What need done?"
	// Turn 3: user "Who are you?"
	// Accompanied by optional trailing empty assistant prefill
	msgs := []any{
		map[string]any{"role": "user", "content": "Hi"},
		map[string]any{"role": "assistant", "content": "Ready. What need done?"},
		map[string]any{"role": "user", "content": "Who are you?"},
		map[string]any{"role": "assistant", "content": ""}, // prefill placeholder
	}

	contents, _ := convertOpenAIMessagesToGemini(msgs, "gemini-3.8-flash")
	if len(contents) != 3 {
		t.Fatalf("expected 3 turns (empty assistant prefill stripped), got %d: %#v", len(contents), contents)
	}

	last := contents[len(contents)-1]
	if last["role"] != "user" {
		t.Errorf("expected last turn to be user, got %v", last["role"])
	}
	lastParts := last["parts"].([]map[string]any)
	if lastParts[0]["text"] != "Who are you?" {
		t.Errorf("expected 'Who are you?', got %v", lastParts[0]["text"])
	}

	// Also verify array content support: [{"type":"text","text":"Who are you?"}]
	msgsArrayContent := []any{
		map[string]any{"role": "user", "content": "Hi"},
		map[string]any{"role": "assistant", "content": "Ready. What need done?"},
		map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": "Who are you?"},
		}},
	}
	contents2, _ := convertOpenAIMessagesToGemini(msgsArrayContent, "gemini-3.8-flash")
	if len(contents2) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(contents2))
	}
	last2 := contents2[len(contents2)-1]
	if last2["role"] != "user" {
		t.Errorf("expected last turn to be user, got %v", last2["role"])
	}
	lastParts2 := last2["parts"].([]map[string]any)
	if !reflect.DeepEqual(lastParts2[0]["text"], "Who are you?") {
		t.Errorf("expected 'Who are you?', got %v", lastParts2[0]["text"])
	}

	// Verify multi-turn tool calling:
	// Turn 1: user "list files"
	// Turn 2: assistant calls tool "read" with ID "call_abc"
	// Turn 3: tool returns "content of dir"
	toolHistory := []any{
		map[string]any{"role": "user", "content": "list files"},
		map[string]any{
			"role":    "assistant",
			"content": nil,
			"tool_calls": []any{
				map[string]any{
					"id":   "call_abc",
					"type": "function",
					"function": map[string]any{
						"name":      "read",
						"arguments": `{"path":"."}`,
					},
				},
			},
		},
		map[string]any{
			"role":         "tool",
			"tool_call_id": "call_abc",
			"content":      "file1.txt\nfile2.txt",
		},
	}
	contents3, _ := convertOpenAIMessagesToGemini(toolHistory, "gemini-3.8-flash")
	if len(contents3) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(contents3))
	}
	// Model turn must have functionCall, and thoughtSignature on the PART (not inside functionCall)
	modelParts := contents3[1]["parts"].([]map[string]any)
	fc, ok := modelParts[0]["functionCall"].(map[string]any)
	if !ok {
		t.Fatalf("expected functionCall, got %#v", modelParts[0])
	}
	if fc["name"] != "read" {
		t.Errorf("expected functionCall name 'read', got %#v", fc)
	}
	if fc["thoughtSignature"] != nil {
		t.Errorf("thoughtSignature must NOT be inside functionCall, but was found: %#v", fc["thoughtSignature"])
	}
	sig, ok := modelParts[0]["thoughtSignature"].(string)
	if !ok || sig == "" {
		t.Errorf("expected thoughtSignature on part, got %#v", modelParts[0]["thoughtSignature"])
	}

	// Tool response turn must have functionResponse with name "read" (matching call), NOT "tool"
	toolParts := contents3[2]["parts"].([]map[string]any)
	fr, ok := toolParts[0]["functionResponse"].(map[string]any)
	if !ok {
		t.Fatalf("expected functionResponse, got %#v", toolParts[0])
	}
	if fr["name"] != "read" {
		t.Errorf("expected functionResponse name 'read', got %v", fr["name"])
	}
	if fr["id"] != "call_abc" {
		t.Errorf("expected functionResponse id 'call_abc', got %v", fr["id"])
	}
}
