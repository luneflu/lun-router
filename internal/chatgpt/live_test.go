package chatgpt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These tests drive a real browser and reach the live ChatGPT endpoint, so they
// are opt-in:
//
//	CHATGPT_LIVE_TESTS=1 go test -run TestLive -v ./internal/chatgpt
//
// They exist because the provider must survive the browser being killed and the
// router being restarted; both previously left the client permanently unable to
// serve requests.

func liveClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("CHATGPT_LIVE_TESTS") != "1" {
		t.Skip("set CHATGPT_LIVE_TESTS=1 to run live browser tests")
	}
	abs, err := filepath.Abs("../../data")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(Config{DataDir: abs})
	if err := c.SetEnabled(true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func liveTurn(t *testing.T, c *Client) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 280*time.Second)
	defer cancel()
	if err := c.EnsureReady(ctx); err != nil {
		return err
	}
	_, err := c.runTurnWithRecovery(ctx, "Reply with exactly: PONG", nil)
	return err
}

// TestLiveBrowserRespawn kills the browser process and asserts the next request
// relaunches it and still completes.
func TestLiveBrowserRespawn(t *testing.T) {
	c := liveClient(t)

	if err := liveTurn(t, c); err != nil {
		t.Fatalf("initial turn failed: %v", err)
	}

	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		t.Fatal("expected a launched browser process")
	}
	oldPID := cmd.Process.Pid
	t.Logf("initial browser pid=%d", oldPID)

	// Simulate the browser dying out from under the router.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill browser: %v", err)
	}
	_, _ = cmd.Process.Wait()

	// Give the read loop a moment to observe the dropped socket.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		c.readyMu.Lock()
		ready := c.ready
		c.readyMu.Unlock()
		if !ready {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	c.readyMu.Lock()
	stillReady := c.ready
	c.readyMu.Unlock()
	if stillReady {
		t.Fatalf("readiness was not invalidated after the browser died")
	}

	// The next request must transparently respawn the browser.
	start := time.Now()
	if err := liveTurn(t, c); err != nil {
		t.Fatalf("turn after browser death failed: %v", err)
	}
	t.Logf("recovered in %v", time.Since(start).Round(time.Millisecond))

	c.mu.Lock()
	newCmd := c.cmd
	c.mu.Unlock()
	if newCmd == nil || newCmd.Process == nil {
		t.Fatal("expected the browser to be relaunched")
	}
	newPID := newCmd.Process.Pid
	if newPID == oldPID {
		t.Fatalf("expected a new browser pid, still %d", oldPID)
	}
	if !processAlive(newPID) {
		t.Fatalf("relaunched browser pid %d is not running", newPID)
	}
	t.Logf("respawned browser pid=%d", newPID)
}

// TestLiveAttachToExistingBrowser drops the CDP connection but leaves the browser
// running, mimicking a router restart, and asserts the next request reattaches
// rather than failing to bind the debug port.
func TestLiveAttachToExistingBrowser(t *testing.T) {
	c := liveClient(t)

	if err := liveTurn(t, c); err != nil {
		t.Fatalf("initial turn failed: %v", err)
	}

	c.mu.Lock()
	cmd := c.cmd
	var pid int
	if cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	c.mu.Unlock()

	// Drop only the socket; the browser keeps running on the debug port. When the
	// browser was attached to rather than launched, c.cmd is nil and there is no
	// pid to compare, so only the reconnect and the turn are asserted.
	c.teardownConnection()

	start := time.Now()
	if err := liveTurn(t, c); err != nil {
		t.Fatalf("turn after reattach failed: %v", err)
	}
	t.Logf("reattached in %v", time.Since(start).Round(time.Millisecond))

	c.mu.Lock()
	after := c.cmd
	c.mu.Unlock()
	if pid != 0 {
		if after == nil || after.Process == nil {
			t.Fatal("expected the same browser to still be tracked")
		}
		if after.Process.Pid != pid {
			t.Fatalf("expected to reuse browser pid %d, got %d", pid, after.Process.Pid)
		}
	}
}

// TestLiveColdStartFirstRequest asserts the very first request after construction
// succeeds, which is where the original cold-start 502 appeared.
func TestLiveColdStartFirstRequest(t *testing.T) {
	c := liveClient(t)

	start := time.Now()
	if err := liveTurn(t, c); err != nil {
		t.Fatalf("cold first request failed: %v", err)
	}
	t.Logf("cold first request completed in %v", time.Since(start).Round(time.Millisecond))
}
