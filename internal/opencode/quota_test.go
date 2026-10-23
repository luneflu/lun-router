package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetAccountUsageParsesQuotaWindows(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/zen/go/v1/usage" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer st_access" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("x-opencode-org-id") != "org_test" {
			t.Errorf("org = %q", r.Header.Get("x-opencode-org-id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usage": map[string]any{
				"rolling": map[string]any{"status": "ok", "percent": 12.4, "resetsAt": "2026-10-23T19:25:51.000Z"},
				"weekly":  map[string]any{"status": "ok", "percent": 0, "resetsAt": "2026-10-26T00:00:00.000Z"},
				"monthly": map[string]any{"status": "ok", "percent": 100, "resetsAt": "2026-11-23T14:22:26.000Z"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, nil)
	client.SetAccountStore(newTestAccountStore(t, activeAccount()))

	usage, err := client.GetAccountUsage(context.Background(), activeAccount(), false)
	if err != nil {
		t.Fatalf("GetAccountUsage: %v", err)
	}
	if usage.Plan != "OpenCode Go" || usage.Email != "test@example.com" {
		t.Errorf("unexpected usage metadata: %+v", usage)
	}
	if len(usage.Quotas) != 3 {
		t.Fatalf("expected 3 quota windows, got %d", len(usage.Quotas))
	}
	rolling := usage.Quotas["rolling"]
	if rolling.Name != "Rolling" || rolling.Used != 12 || rolling.Total != 100 {
		t.Errorf("unexpected rolling quota: %+v", rolling)
	}
	if rolling.RemainingPercentage != 87.6 {
		t.Errorf("remaining = %v, want 87.6", rolling.RemainingPercentage)
	}
	if usage.Quotas["monthly"].RemainingPercentage != 0 {
		t.Errorf("monthly remaining = %v, want 0", usage.Quotas["monthly"].RemainingPercentage)
	}

	// Cached reads must not hit upstream again.
	if _, err := client.GetAccountUsage(context.Background(), activeAccount(), false); err != nil {
		t.Fatalf("cached GetAccountUsage: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 upstream call, got %d", got)
	}

	if _, err := client.GetAccountUsage(context.Background(), activeAccount(), true); err != nil {
		t.Fatalf("forced GetAccountUsage: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 upstream calls after force, got %d", got)
	}
}

func TestGetAllUsageAcrossAccounts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usage": map[string]any{
				"rolling": map[string]any{"status": "ok", "percent": 50, "resetsAt": "2026-10-23T19:25:51.000Z"},
			},
		})
	}))
	defer srv.Close()

	store := newTestAccountStore(t,
		&Account{ID: "user_a", Email: "a@example.com", AccessToken: "st_a", OrgID: "org_a", ExpiresAt: time.Now().Add(time.Hour), IsActive: true},
		&Account{ID: "user_b", Email: "b@example.com", AccessToken: "st_b", OrgID: "org_b", ExpiresAt: time.Now().Add(time.Hour), IsActive: true},
	)
	client := NewClient(srv.URL, nil)
	client.SetAccountStore(store)

	usages, err := client.GetAllUsage(context.Background(), true)
	if err != nil {
		t.Fatalf("GetAllUsage: %v", err)
	}
	if len(usages) != 2 {
		t.Fatalf("expected 2 usages, got %d", len(usages))
	}
	if usages["user_a"] == nil || usages["user_b"] == nil {
		t.Fatalf("missing account usage: %+v", usages)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 upstream calls, got %d", got)
	}
}
