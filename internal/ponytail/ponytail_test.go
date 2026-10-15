package ponytail

import (
	"os"
	"strings"
	"testing"
)

func TestPonytailDefaults(t *testing.T) {
	tmp, err := os.MkdirTemp("", "ponytail-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s, err := NewService(tmp)
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}
	defer s.Close()

	st := s.GetStatus()
	if !st.Enabled {
		t.Errorf("expected default enabled")
	}
	if st.Mode != "full" {
		t.Errorf("expected mode full, got %s", st.Mode)
	}
	if len(st.ActivePrompt) == 0 {
		t.Errorf("expected non-empty active prompt")
	}
	if len(st.AvailableModes) < 4 {
		t.Errorf("expected at least 4 modes")
	}
}

func TestPonytailApplyOpenAI(t *testing.T) {
	tmp, err := os.MkdirTemp("", "ponytail-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s, err := NewService(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Write a rate limiter in Go",
			},
		},
	}

	applied := s.Apply(body, "", "")
	if !applied {
		t.Fatalf("expected ponytail to be applied")
	}

	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	first := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("expected prepended system message, got %v", first["role"])
	}
	if !strings.Contains(first["content"].(string), "You are a lazy senior developer") {
		t.Errorf("expected ponytail prompt in system message")
	}
}

func TestPonytailApplyExistingSystem(t *testing.T) {
	tmp, err := os.MkdirTemp("", "ponytail-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s, err := NewService(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "system",
				"content": "You are a coding assistant.",
			},
			map[string]any{
				"role":    "user",
				"content": "Build an authentication service",
			},
		},
	}

	applied := s.Apply(body, "", "review")
	if !applied {
		t.Fatalf("expected ponytail to be applied")
	}

	msgs := body["messages"].([]any)
	first := msgs[0].(map[string]any)
	content := first["content"].(string)

	if !strings.Contains(content, "You are a coding assistant.") {
		t.Errorf("expected original system prompt preserved")
	}
	if !strings.Contains(content, "Review a change like the senior developer") {
		t.Errorf("expected review prompt appended")
	}
}

func TestPonytailBypass(t *testing.T) {
	tmp, err := os.MkdirTemp("", "ponytail-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s, err := NewService(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "hello",
			},
		},
	}

	applied := s.Apply(body, "off", "")
	if applied {
		t.Errorf("expected ponytail to NOT be applied when header is off")
	}
}
