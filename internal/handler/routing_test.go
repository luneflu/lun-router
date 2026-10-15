package handler

import "testing"

func TestAntigravityShouldHandle(t *testing.T) {
	// OpenCode Free upstream serves these too, so bare IDs belong to OpenCode.
	shared := []string{"gemini-3.8-flash", "claude-sonnet-5-5"}
	// Antigravity-only IDs resolve to Antigravity without a prefix.
	agOnly := "gemini-3.8-flash-high"

	cases := []struct {
		model    string
		opencode []string
		want     bool
	}{
		{shared[0], shared, false},
		{shared[1], shared, false},
		{agOnly, shared, true},
		{agOnly, nil, true},
		{"big-pickle", shared, false},
	}
	for _, c := range cases {
		if got := antigravityShouldHandle(c.model, c.opencode); got != c.want {
			t.Errorf("antigravityShouldHandle(%q, %v) = %v, want %v", c.model, c.opencode, got, c.want)
		}
	}
}

func TestSplitProviderPrefix(t *testing.T) {
	cases := []struct {
		model    string
		provider string
		id       string
		ok       bool
	}{
		{"opencode/space-bunny-free", "opencode", "space-bunny-free", true},
		{"antigravity/gemini-3.8-flash", "antigravity", "gemini-3.8-flash", true},
		// "ag/" is not a namespace
		{"ag/gemini-3.8-flash", "", "ag/gemini-3.8-flash", false},
		{"big-pickle", "", "big-pickle", false},
	}
	for _, c := range cases {
		provider, id, ok := splitProviderPrefix(c.model)
		if provider != c.provider || id != c.id || ok != c.ok {
			t.Errorf("splitProviderPrefix(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.model, provider, id, ok, c.provider, c.id, c.ok)
		}
	}
}

func TestIsAntigravityFamilyModel(t *testing.T) {
	hidden := []string{"gemini-3.8-flash", "claude-opus-5", "gemini-4-argon", "claude-fable-5"}
	kept := []string{"big-pickle", "gpt-5.2", "grok-4.7", "deepseek-v4-pro", "mimo-v2.6-flash-free"}

	for _, id := range hidden {
		if !isAntigravityFamilyModel(id) {
			t.Errorf("expected %q to be hidden from OpenCode Free", id)
		}
	}
	for _, id := range kept {
		if isAntigravityFamilyModel(id) {
			t.Errorf("expected %q to stay in OpenCode Free", id)
		}
	}
}
