package zai

import (
	"strings"
	"testing"

	"lun-router/internal/chatgpt"
)

// TestResolveModel covers the id handling a client actually sends: a bare id, a
// prefixed id, "auto", and an unknown id.
func TestResolveModel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"x-preview-l", "x-preview-l"},
		{"zai/x-preview-l", "x-preview-l"},
		{"glm-5.3", "glm-5.3"},
		{"zai/glm-5.3", "glm-5.3"},
		{"auto", DefaultModel},
		{"", DefaultModel},
		{"totally-unknown", DefaultModel},
	}
	for _, tc := range cases {
		if got := ResolveModel(tc.in).ID; got != tc.want {
			t.Errorf("ResolveModel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestCatalogGuestFlag guards the set confirmed live by calling every catalog
// id with an anonymous token: only x-preview-l and glm-4.7 answered, the other
// eight returned 403 "Model not available for current user level". If this set
// drifts, the router either advertises models that never respond or hides ones
// that work.
func TestCatalogGuestFlag(t *testing.T) {
	wantGuest := map[string]bool{"x-preview-l": true, "glm-4.7": true}
	for _, m := range Catalog {
		if wantGuest[m.ID] != m.Guest {
			t.Errorf("%s: Guest = %v, want %v", m.ID, m.Guest, wantGuest[m.ID])
		}
	}
	if len(UsableModels(false)) != 2 {
		t.Errorf("anonymous session should see 2 models, got %d", len(UsableModels(false)))
	}
	if len(UsableModels(true)) != len(Catalog) {
		t.Errorf("signed-in session should see the whole catalog")
	}
}

// TestIsCatalogModel checks prefix handling for the dispatcher.
func TestIsCatalogModel(t *testing.T) {
	if !IsCatalogModel("zai/x-preview-l") {
		t.Error("prefixed catalog id should match")
	}
	if !IsCatalogModel("x-preview-l") {
		t.Error("bare catalog id should match")
	}
	if IsCatalogModel("gpt-6") {
		t.Error("foreign id must not match")
	}
}

// TestThinkOptions checks reasoning negotiation, which decides whether the
// request asks for the thinking phase at all.
func TestThinkOptions(t *testing.T) {
	reasoning := LookupModel("x-preview-l")

	think, effort := thinkOptions(map[string]any{}, reasoning)
	if !think || effort != "max" {
		t.Errorf("default = (%v, %q), want (true, max)", think, effort)
	}

	think, effort = thinkOptions(map[string]any{"reasoning_effort": "low"}, reasoning)
	if !think || effort != "low" {
		t.Errorf("low = (%v, %q)", think, effort)
	}

	think, _ = thinkOptions(map[string]any{"reasoning_effort": "none"}, reasoning)
	if think {
		t.Error("reasoning_effort none must disable thinking")
	}

	think, _ = thinkOptions(map[string]any{"thinking": map[string]any{"type": "disabled"}}, reasoning)
	if think {
		t.Error("thinking.type disabled must disable thinking")
	}

	// A non-reasoning model never asks for reasoning.
	think, effort = thinkOptions(map[string]any{}, Model{Reasoning: false})
	if think || effort != "" {
		t.Errorf("non-reasoning model = (%v, %q)", think, effort)
	}
}

// TestBuildPromptFlattensHistory verifies the transcript carries system,
// assistant and tool turns, since the upstream request only takes one message.
func TestBuildPromptFlattensHistory(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "be terse"},
			map[string]any{"role": "user", "content": "read the file"},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"function": map[string]any{"name": "read", "arguments": `{"filePath":"README.md"}`},
					},
				},
			},
			map[string]any{"role": "tool", "name": "read", "content": "# Title"},
		},
	}
	got := buildPrompt(body)
	for _, want := range []string{"[system] be terse", "read the file", "called tool read", "Tool result (read): # Title"} {
		if !contains(got, want) {
			t.Errorf("prompt missing %q\n%s", want, got)
		}
	}
}

// TestParseBareCall covers the shape the service delivers once it has stripped
// the <tool_call> wrapper - plain JSON with no tags.
func TestParseBareCall(t *testing.T) {
	call := parseBareCall(`{"name": "read", "arguments": {"filePath": "README.md"}}`)
	if call == nil {
		t.Fatal("bare call not parsed")
	}
	if call.Name != "read" {
		t.Errorf("name = %q", call.Name)
	}
	if call.Arguments["filePath"] != "README.md" {
		t.Errorf("arguments = %v", call.Arguments)
	}

	// Nested OpenAI form.
	nested := parseBareCall(`{"function": {"name": "bash", "arguments": "{\"command\":\"ls\"}"}}`)
	if nested == nil || nested.Name != "bash" || nested.Arguments["command"] != "ls" {
		t.Errorf("nested call = %+v", nested)
	}

	if parseBareCall("just prose") != nil {
		t.Error("prose must not parse as a tool call")
	}
	if parseBareCall(`{"name": "read"}`) != nil {
		t.Error("a call with no arguments must not be accepted")
	}
}

// TestToolPreambleNamesAdvertisedTools checks the preamble only teaches tools the
// caller actually advertised. The ChatGPT provider shipped a hardcoded example
// naming a tool the client did not have, which is what broke shell execution.
func TestToolPreambleNamesAdvertisedTools(t *testing.T) {
	specs := []chatgpt.ToolSpec{
		{Name: "read", Description: "Read a file", Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"filePath": map[string]any{"type": "string"}},
			"required":   []any{"filePath"},
		}},
	}
	got := toolPreamble(specs)
	if !contains(got, "- read") {
		t.Error("advertised tool missing from preamble")
	}
	if !contains(got, `"filePath"`) {
		t.Error("argument name missing from preamble")
	}
	if contains(got, "shell") {
		t.Error("preamble taught a tool the client never advertised")
	}
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
