package ponytail

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
			map[string]any{"role": "user", "content": "Write a rate limiter in Go"},
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

func TestPonytailDefaults(t *testing.T) {
	s := newTestService(t)

	st := s.GetStatus()
	if !st.Enabled {
		t.Errorf("expected default enabled")
	}
	if st.Mode != "full" {
		t.Errorf("expected mode full, got %s", st.Mode)
	}
	if st.ActivePrompt != DefaultPonytailFullPrompt {
		t.Errorf("expected the built-in full rule to be active")
	}
	if len(st.AvailableModes) != 5 {
		t.Errorf("expected 5 modes, got %d", len(st.AvailableModes))
	}
}

func TestPonytailApplyPrependsSystemMessage(t *testing.T) {
	s := newTestService(t)
	body := userBody()

	if !s.Apply(body, "", "") {
		t.Fatalf("expected ponytail to be applied")
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
	if !strings.Contains(content, "[System directive: Lazy Senior Developer Guidance (full)]") {
		t.Errorf("expected the directive header on the prepended message, got %q", content)
	}
	if !strings.Contains(content, DefaultPonytailFullPrompt) {
		t.Errorf("expected the built-in full rule in the system message")
	}
	if !strings.Contains(content, "Standalone / testing queries") {
		t.Errorf("expected the standalone tail on the injected instruction")
	}
}

func TestPonytailApplyAppendsToExistingSystem(t *testing.T) {
	s := newTestService(t)
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are a helpful assistant."},
			map[string]any{"role": "user", "content": "Build a queue"},
		},
	}

	if !s.Apply(body, "", "review") {
		t.Fatalf("expected ponytail to be applied")
	}

	content := firstMessage(t, body)["content"].(string)
	if !strings.Contains(content, "You are a helpful assistant.") {
		t.Errorf("expected the original system prompt preserved")
	}
	if !strings.Contains(content, DefaultPonytailReviewPrompt) {
		t.Errorf("expected the review rule appended")
	}
	if !strings.Contains(content, "[System directive: Lazy Senior Developer Guidance (review)]") {
		t.Errorf("expected the review header on the appended instruction")
	}
}

func TestPonytailApplyAnthropicSystemString(t *testing.T) {
	s := newTestService(t)
	body := map[string]any{
		"system":   "You are Claude.",
		"messages": []any{},
	}

	if !s.Apply(body, "", "lite") {
		t.Fatalf("expected ponytail to be applied to the system field")
	}

	sys := body["system"].(string)
	if !strings.Contains(sys, "You are Claude.") || !strings.Contains(sys, DefaultPonytailLitePrompt) {
		t.Errorf("expected the lite rule appended to system, got %q", sys)
	}
}

func TestPonytailApplyModeHeaders(t *testing.T) {
	cases := []struct {
		headerPonytail string
		headerMode     string
		want           string
	}{
		{"", "", DefaultPonytailFullPrompt},
		{"", "full", DefaultPonytailFullPrompt},
		{"", "lite", DefaultPonytailLitePrompt},
		{"", "ultra", DefaultPonytailUltraPrompt},
		{"", "review", DefaultPonytailReviewPrompt},
		{"lite", "", DefaultPonytailLitePrompt},
		{"ultra", "", DefaultPonytailUltraPrompt},
		{"review", "", DefaultPonytailReviewPrompt},
	}

	for _, tc := range cases {
		s := newTestService(t)
		body := userBody()

		if !s.Apply(body, tc.headerPonytail, tc.headerMode) {
			t.Fatalf("expected injection for header %q / mode %q", tc.headerPonytail, tc.headerMode)
		}
		content := firstMessage(t, body)["content"].(string)
		if !strings.Contains(content, tc.want) {
			t.Errorf("header %q / mode %q: wrong rule injected", tc.headerPonytail, tc.headerMode)
		}
	}
}

func TestPonytailApplyCustomPrompt(t *testing.T) {
	s := newTestService(t)

	if _, err := s.UpdateConfig(nil, strPtr("custom"), strPtr("Return one diff only.")); err != nil {
		t.Fatalf("failed to set custom mode: %v", err)
	}

	body := userBody()
	if !s.Apply(body, "", "") {
		t.Fatalf("expected injection in custom mode")
	}

	content := firstMessage(t, body)["content"].(string)
	if !strings.Contains(content, "Return one diff only.") {
		t.Errorf("expected the custom prompt injected, got %q", content)
	}
	if s.GetStatus().ActivePrompt != "Return one diff only." {
		t.Errorf("expected the custom prompt reported as active")
	}
}

func TestPonytailBypass(t *testing.T) {
	s := newTestService(t)
	body := userBody()

	if s.Apply(body, "off", "") {
		t.Errorf("expected no injection when the header is off")
	}
	if len(body["messages"].([]any)) != 1 {
		t.Errorf("expected the body untouched")
	}
}

func TestPonytailApplyDisabled(t *testing.T) {
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
func TestPonytailIgnoresLegacyStoredRules(t *testing.T) {
	dir := t.TempDir()
	legacy := `{
  "enabled": true,
  "mode": "full",
  "auto_update": true,
  "check_interval_hours": 6,
  "last_sync": "2026-10-15T22:49:46.401653+07:00",
  "upstream_commit": "abc12345",
  "upstream_commit_msg": "chore: sync SKILL.md copies [skip ci]",
  "upstream_commit_date": "2026-10-13T18:08:08Z",
  "schema_version": 2,
  "rules": {
    "full": "# Ponytail\n\nstale upstream skill text",
    "lite": "stale",
    "ultra": "stale",
    "review": "stale"
  }
}`
	path := filepath.Join(dir, "ponytail.json")
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := NewService(dir)
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	st := s.GetStatus()
	if st.ActivePrompt != DefaultPonytailFullPrompt {
		t.Errorf("stored rules must not win, got %q", st.ActivePrompt)
	}
	if !st.Enabled || st.Mode != "full" {
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
	if !onDisk.Enabled || onDisk.Mode != "full" {
		t.Errorf("expected user state persisted, got %+v", onDisk)
	}
}

func TestPonytailRejectsUnknownMode(t *testing.T) {
	s := newTestService(t)

	if _, err := s.UpdateConfig(nil, strPtr("review"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateConfig(nil, strPtr("nonsense"), nil); err != nil {
		t.Fatal(err)
	}
	if mode := s.GetStatus().Mode; mode != "review" {
		t.Errorf("expected the unknown mode ignored, got %s", mode)
	}
}
