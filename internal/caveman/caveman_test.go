package caveman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

func newTestService(t *testing.T) *Service {
	t.Helper()
	s, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}
	return s
}

func userBody() map[string]any {
	return map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Why is React re-rendering?"},
		},
	}
}

func firstMessage(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected messages in body, got %#v", body["messages"])
	}
	msg, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first message to be a map, got %#v", msgs[0])
	}
	return msg
}

func TestCavemanDefaults(t *testing.T) {
	s := newTestService(t)

	st := s.GetStatus()
	if !st.Enabled {
		t.Errorf("expected enabled by default")
	}
	if st.Mode != "caveman" {
		t.Errorf("expected mode caveman, got %s", st.Mode)
	}
	if st.ActivePrompt != DefaultCavemanPrompt {
		t.Errorf("expected the built-in caveman rule to be active")
	}
	if len(st.AvailableModes) != 4 {
		t.Errorf("expected 4 modes, got %d", len(st.AvailableModes))
	}
}

func TestCavemanApplyPrependsSystemMessage(t *testing.T) {
	s := newTestService(t)
	body := userBody()

	if !s.Apply(body, "", "") {
		t.Fatalf("expected caveman to be applied")
	}

	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	first := firstMessage(t, body)
	if first["role"] != "system" {
		t.Errorf("expected prepended system message, got %v", first["role"])
	}
	content := first["content"].(string)
	if !strings.Contains(content, "[System directive: Response style]") {
		t.Errorf("expected the directive header on the prepended message, got %q", content)
	}
	if !strings.Contains(content, DefaultCavemanPrompt) {
		t.Errorf("expected the built-in caveman rule in the system message")
	}
}

func TestCavemanApplyAppendsToExistingSystem(t *testing.T) {
	s := newTestService(t)
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are a helpful assistant."},
			map[string]any{"role": "user", "content": "Help me fix this bug"},
		},
	}

	if !s.Apply(body, "", "ultracave") {
		t.Fatalf("expected caveman to be applied")
	}

	content := firstMessage(t, body)["content"].(string)
	if !strings.Contains(content, "You are a helpful assistant.") {
		t.Errorf("expected the original system prompt preserved")
	}
	if !strings.Contains(content, DefaultUltracavePrompt) {
		t.Errorf("expected the ultracave rule appended")
	}
}

func TestCavemanApplyAnthropicSystemString(t *testing.T) {
	s := newTestService(t)
	body := map[string]any{
		"system":   "You are Claude.",
		"messages": []any{},
	}

	if !s.Apply(body, "", "megacave") {
		t.Fatalf("expected caveman to be applied to the system field")
	}

	sys := body["system"].(string)
	if !strings.Contains(sys, "You are Claude.") || !strings.Contains(sys, DefaultMegacavePrompt) {
		t.Errorf("expected the megacave rule appended to system, got %q", sys)
	}
}

func TestCavemanApplyModeHeaders(t *testing.T) {
	cases := []struct {
		headerCaveman string
		headerMode    string
		want          string
	}{
		{"", "", DefaultCavemanPrompt},
		{"", "ultracave", DefaultUltracavePrompt},
		{"", "megacave", DefaultMegacavePrompt},
		{"ultracave", "", DefaultUltracavePrompt},
		{"ultra", "", DefaultUltracavePrompt},
		{"megacave", "", DefaultMegacavePrompt},
		{"wenyan", "", DefaultMegacavePrompt},
	}

	for _, tc := range cases {
		s := newTestService(t)
		body := userBody()

		if !s.Apply(body, tc.headerCaveman, tc.headerMode) {
			t.Fatalf("expected injection for header %q / mode %q", tc.headerCaveman, tc.headerMode)
		}
		content := firstMessage(t, body)["content"].(string)
		if !strings.Contains(content, tc.want) {
			t.Errorf("header %q / mode %q: wrong rule injected", tc.headerCaveman, tc.headerMode)
		}
	}
}

func TestCavemanApplyCustomPrompt(t *testing.T) {
	s := newTestService(t)

	if _, err := s.UpdateConfig(nil, strPtr("custom"), strPtr("Answer in haiku.")); err != nil {
		t.Fatalf("failed to set custom mode: %v", err)
	}

	body := userBody()
	if !s.Apply(body, "", "") {
		t.Fatalf("expected injection in custom mode")
	}

	content := firstMessage(t, body)["content"].(string)
	if !strings.Contains(content, "Answer in haiku.") {
		t.Errorf("expected the custom prompt injected, got %q", content)
	}
	if s.GetStatus().ActivePrompt != "Answer in haiku." {
		t.Errorf("expected the custom prompt reported as active")
	}
}

func TestCavemanApplyBypass(t *testing.T) {
	s := newTestService(t)
	body := userBody()

	if s.Apply(body, "off", "") {
		t.Errorf("expected no injection when the header is off")
	}
	if len(body["messages"].([]any)) != 1 {
		t.Errorf("expected the body untouched")
	}
}

func TestCavemanApplyDisabled(t *testing.T) {
	s := newTestService(t)

	if _, err := s.UpdateConfig(boolPtr(false), nil, nil); err != nil {
		t.Fatalf("failed to disable: %v", err)
	}
	if s.Apply(userBody(), "", "") {
		t.Errorf("expected no injection while disabled")
	}
	if !s.Apply(userBody(), "on", "") {
		t.Errorf("expected the on header to force injection while disabled")
	}
}

// The point of the change: built-in rules live in the binary, so a data dir written by
// an older build cannot pin the injected text.
func TestCavemanIgnoresLegacyStoredRules(t *testing.T) {
	dir := t.TempDir()
	legacy := `{
  "enabled": true,
  "mode": "caveman",
  "auto_update": true,
  "check_interval_hours": 6,
  "last_sync": "2026-10-15T22:49:46.401653+07:00",
  "upstream_commit": "d262ab3882952a59312ff3537cd86d894523c5ab",
  "upstream_commit_msg": "chore: sync SKILL.md copies [skip ci]",
  "upstream_commit_date": "2026-10-13T18:08:08Z",
  "schema_version": 2,
  "rules": {
    "caveman": "# caveman\n\nstale upstream skill text",
    "ultracave": "# ultracave\n\nstale",
    "megacave": "# megacave\n\nstale"
  }
}`
	path := filepath.Join(dir, "caveman.json")
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := NewService(dir)
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	st := s.GetStatus()
	if st.ActivePrompt != DefaultCavemanPrompt {
		t.Errorf("stored rules must not win, got %q", st.ActivePrompt)
	}
	if !st.Enabled || st.Mode != "caveman" {
		t.Errorf("expected user state preserved, got enabled=%v mode=%s", st.Enabled, st.Mode)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"rules", "auto_update", "upstream_commit", "last_sync", "check_interval_hours", "schema_version"} {
		if strings.Contains(string(raw), `"`+key+`"`) {
			t.Errorf("expected legacy key %q pruned from disk, file: %s", key, raw)
		}
	}

	var onDisk Config
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if !onDisk.Enabled || onDisk.Mode != "caveman" {
		t.Errorf("expected user state persisted, got %+v", onDisk)
	}
}

func TestCavemanRejectsUnknownMode(t *testing.T) {
	s := newTestService(t)

	if _, err := s.UpdateConfig(nil, strPtr("ultracave"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateConfig(nil, strPtr("nonsense"), nil); err != nil {
		t.Fatal(err)
	}
	if mode := s.GetStatus().Mode; mode != "ultracave" {
		t.Errorf("expected the unknown mode ignored, got %s", mode)
	}
}

func TestCavemanTailReinforcement(t *testing.T) {
	s := newTestService(t)

	// Case 1: Short 1-turn chat: user message should NOT have tail reminder
	bodySingle := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello world"},
		},
	}
	if !s.Apply(bodySingle, "", "") {
		t.Fatal("expected apply true")
	}
	msgs := bodySingle["messages"].([]any)
	userMsg := msgs[1].(map[string]any)
	if strings.Contains(userMsg["content"].(string), "[System directive reminder:") {
		t.Errorf("expected no tail reminder on short single user message, got %v", userMsg["content"])
	}

	// Case 2: Multi-turn chat: last user message MUST have tail reminder
	bodyMulti := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "How do I do X?"},
			map[string]any{"role": "assistant", "content": "Here is how you do X with many words..."},
			map[string]any{"role": "user", "content": "Now explain step 2"},
		},
	}
	if !s.Apply(bodyMulti, "", "ultracave") {
		t.Fatal("expected apply true")
	}
	msgsMulti := bodyMulti["messages"].([]any)
	lastUser := msgsMulti[len(msgsMulti)-1].(map[string]any)
	lastContent := lastUser["content"].(string)
	if !strings.Contains(lastContent, "[System directive reminder: Ultracave mode active.") {
		t.Errorf("expected ultracave tail reminder in last user message, got %v", lastContent)
	}

	// Case 3: Long prompt (> 2000 chars) single user message: MUST have tail reminder
	longContent := strings.Repeat("very long context information ", 100)
	bodyLong := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": longContent},
		},
	}
	if !s.Apply(bodyLong, "", "caveman") {
		t.Fatal("expected apply true")
	}
	msgsLong := bodyLong["messages"].([]any)
	longUser := msgsLong[len(msgsLong)-1].(map[string]any)
	if !strings.Contains(longUser["content"].(string), "[System directive reminder: Caveman mode active.") {
		t.Errorf("expected caveman tail reminder in long user prompt")
	}

	// Case 4: Structured content parts array
	bodyArray := map[string]any{
		"messages": []any{
			map[string]any{"role": "assistant", "content": "Hi"},
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "Check this"},
				},
			},
		},
	}
	if !s.Apply(bodyArray, "", "caveman") {
		t.Fatal("expected apply true")
	}
	msgsArray := bodyArray["messages"].([]any)
	lastPartUser := msgsArray[len(msgsArray)-1].(map[string]any)
	parts := lastPartUser["content"].([]any)
	textPart := parts[0].(map[string]any)
	if !strings.Contains(textPart["text"].(string), "[System directive reminder: Caveman mode active.") {
		t.Errorf("expected tail reminder in structured text part, got %v", textPart["text"])
	}
}

