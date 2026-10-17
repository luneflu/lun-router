package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"lun-router/internal/zai"
)

// TestZaiStatusEndpoint pins the status contract the provider page consumes. The
// page renders browser installation and live browser RAM from this payload, so a
// silently missing key would leave those widgets stuck at zero.
func TestZaiStatusEndpoint(t *testing.T) {
	zc := zai.NewClient(zai.Config{DataDir: t.TempDir()})
	defer zc.Close()

	compH := NewCompletionHandler(nil, nil, nil, nil, nil, nil, nil)
	compH.SetZai(zc)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/zai/status", nil)
	w := httptest.NewRecorder()
	compH.ZaiStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{
		"enabled", "installed", "browser_path",
		"install_state", "install_progress", "install_error",
		"browser_running", "browser_pid", "browser_procs", "browser_rss_mb",
		"passthrough_tools", "guest_mode",
	} {
		if _, ok := res[key]; !ok {
			t.Errorf("status payload is missing %q", key)
		}
	}

	// With no browser launched the metrics must report a clean idle state rather
	// than a stale pid, because the page shows "Idle" off exactly these values.
	if res["browser_running"] != false {
		t.Errorf("browser_running = %v, want false", res["browser_running"])
	}
	if pid, _ := res["browser_pid"].(float64); pid != 0 {
		t.Errorf("browser_pid = %v, want 0", res["browser_pid"])
	}
	if procs, _ := res["browser_procs"].(float64); procs != 0 {
		t.Errorf("browser_procs = %v, want 0", res["browser_procs"])
	}
	if rss, _ := res["browser_rss_mb"].(float64); rss != 0 {
		t.Errorf("browser_rss_mb = %v, want 0", res["browser_rss_mb"])
	}
}

// TestZaiStatusWithoutClient guards the degraded payload used before the client is
// wired, which the page also has to render.
func TestZaiStatusWithoutClient(t *testing.T) {
	compH := NewCompletionHandler(nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/zai/status", nil)
	w := httptest.NewRecorder()
	compH.ZaiStatus(w, req)

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res["enabled"] != false {
		t.Errorf("expected enabled=false without a client")
	}
	if _, ok := res["error"]; !ok {
		t.Errorf("expected an error field without a client")
	}
}
