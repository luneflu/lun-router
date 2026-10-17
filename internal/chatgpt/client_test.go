package chatgpt

import (
	"strings"
	"testing"
)

func TestModelHelpers(t *testing.T) {
	if !IsCatalogModel("auto") {
		t.Errorf("expected auto to be catalog model")
	}
	if !IsCatalogModel("chatgpt/gpt-6") {
		t.Errorf("expected chatgpt/gpt-6 to be catalog model with prefix")
	}
	if !IsCatalogModel("GPT-6-MINI") {
		t.Errorf("expected case-insensitive match")
	}
	if IsCatalogModel("claude-sonnet-4") {
		t.Errorf("did not expect claude-sonnet-4 to be chatgpt catalog model")
	}

	if !HasPrefix("chatgpt/auto") {
		t.Errorf("expected HasPrefix to be true")
	}
	if HasPrefix("opencode/auto") {
		t.Errorf("expected HasPrefix to be false")
	}

	if CleanModelID("chatgpt/auto") != "auto" {
		t.Errorf("expected clean model ID")
	}
}

func TestExtractPrompt(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "system",
				"content": "You are a helpful assistant.",
			},
			map[string]any{
				"role":    "user",
				"content": "Hello world",
			},
		},
	}

	p := extractPrompt(body, false)
	if p == "" {
		t.Errorf("expected non-empty prompt")
	}
	if !strings.Contains(p, "System instruction: You are a helpful assistant.") {
		t.Errorf("expected system instruction in prompt, got: %q", p)
	}
	if !strings.Contains(p, "User: Hello world") {
		t.Errorf("expected user message in prompt, got: %q", p)
	}
	if !strings.Contains(p, "[API CONTEXT - NOT A WEB CHAT]") {
		t.Errorf("expected API context preamble, got: %q", p)
	}
	if !strings.Contains(p, "Assistant:") {
		t.Errorf("expected trailing Assistant turn, got: %q", p)
	}
}
