package zai

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestMintDoesNotDeadlockOnWarmBrowser guards a real regression: mintCaptcha
// holds browserMu, and ensureBrowser used to call the locking BrowserRunning()
// helper. With no browser yet the nil check short-circuited and everything
// worked, so the bug only appeared on the *second* mint - i.e. the first real
// completion, which hung until the client gave up.
func TestMintDoesNotDeadlockOnWarmBrowser(t *testing.T) {
	// Build through NewClient so the installer (used to locate a browser when
	// the launch path is reached) is populated.
	c := NewClient(Config{DataDir: t.TempDir()})
	c.profileDir = t.TempDir()

	// Simulate a browser that is already attached by pointing the field at a
	// non-nil value. The mint will fail fast (no real CDP connection), which is
	// fine: the point is that it must return rather than block.
	c.browserMu.Lock()
	c.browser = &browser{closed: false}
	c.browserMu.Unlock()

	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer once.Do(func() { close(done) })
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = c.mintCaptcha(ctx)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("mintCaptcha deadlocked while a browser was already attached")
	}
}
