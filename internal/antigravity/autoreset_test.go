package antigravity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTakeExpiredResets(t *testing.T) {
	now := time.Now()
	c := &Client{exhausted: map[string]time.Time{
		"acc1|gemini-3.8-flash":       now.Add(-time.Minute),
		"acc1|gemini_session":         now.Add(-time.Minute),
		"acc1|claude-sonnet-5-5":      now.Add(time.Hour),
		"acc2|claude-session_session": now.Add(-time.Minute),
		"acc2|claude-sonnet-5-5-high": now.Add(-time.Minute),
		"acc3|gemini-3.8-flash":       now.Add(-time.Minute),
		"acc3|claude-opus-5-5":        now.Add(-time.Minute),
	}}

	targets := c.takeExpiredResets(now)

	// acc1: gemini window expired (claude still parked), acc2: claude window,
	// acc3: both families -> one ping per window.
	if len(targets) != 4 {
		t.Fatalf("targets = %+v, want 4 family pings", targets)
	}
	perAccount := map[string]int{}
	for _, target := range targets {
		perAccount[target.accountID]++
	}
	if perAccount["acc1"] != 1 || perAccount["acc2"] != 1 || perAccount["acc3"] != 2 {
		t.Errorf("per-account pings = %v", perAccount)
	}
	if _, ok := c.exhausted["acc1|claude-sonnet-5-5"]; !ok {
		t.Error("unexpired entry must stay parked")
	}
	if len(c.exhausted) != 1 {
		t.Errorf("expired entries not dropped: %v", c.exhausted)
	}
}

func TestAutoResetTogglePersists(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAccountStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	c := NewClient(store, dir)
	if !c.AutoResetEnabled() {
		t.Fatal("auto reset should default on")
	}
	if err := c.SetAutoResetEnabled(false); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "antigravity_settings.json")); err != nil {
		t.Fatalf("settings file: %v", err)
	}

	reloaded := NewClient(store, dir)
	if reloaded.AutoResetEnabled() {
		t.Error("disabled setting did not survive reload")
	}
}
