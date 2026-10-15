package caveman

import (
	"os"
	"strings"
	"testing"
)

func TestCavemanDefaults(t *testing.T) {
	tmp, err := os.MkdirTemp("", "caveman-test-*")
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
	if st.Mode != "caveman" {
		t.Errorf("expected mode caveman, got %s", st.Mode)
	}
	if len(st.ActivePrompt) == 0 {
		t.Errorf("expected non-empty active prompt")
	}
	if len(st.AvailableModes) < 3 {
		t.Errorf("expected at least 3 modes")
	}
}

func TestCavemanApplyOpenAI(t *testing.T) {
	tmp, err := os.MkdirTemp("", "caveman-test-*")
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
				"content": "Why is React re-rendering?",
			},
		},
	}

	applied := s.Apply(body, "", "")
	if !applied {
		t.Fatalf("expected caveman to be applied")
	}

	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	first := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("expected prepended system message, got %v", first["role"])
	}
	if !strings.Contains(first["content"].(string), "Respond terse like smart caveman") {
		t.Errorf("expected caveman prompt in system message")
	}
}

func TestCavemanApplyExistingSystem(t *testing.T) {
	tmp, err := os.MkdirTemp("", "caveman-test-*")
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
				"content": "You are a helpful assistant.",
			},
			map[string]any{
				"role":    "user",
				"content": "Help me fix this bug",
			},
		},
	}

	applied := s.Apply(body, "", "ultracave")
	if !applied {
		t.Fatalf("expected caveman to be applied")
	}

	msgs := body["messages"].([]any)
	first := msgs[0].(map[string]any)
	content := first["content"].(string)

	if !strings.Contains(content, "You are a helpful assistant.") {
		t.Errorf("expected original system prompt preserved")
	}
	if !strings.Contains(content, "Ultracave is caveman with the grammar stripped") {
		t.Errorf("expected ultracave prompt appended")
	}
}

func TestCavemanBypass(t *testing.T) {
	tmp, err := os.MkdirTemp("", "caveman-test-*")
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
		t.Errorf("expected caveman to NOT be applied when header is off")
	}
}
