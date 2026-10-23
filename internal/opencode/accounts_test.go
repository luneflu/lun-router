package opencode

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccountStoreSaveAndSelect(t *testing.T) {
	store, err := NewAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAccountStore: %v", err)
	}

	first := &Account{ID: "user_first", Email: "first@example.com", Server: ConsoleBaseURL, AccessToken: "st_first", RefreshToken: "rt_first", ExpiresAt: time.Now().Add(time.Hour), IsActive: true}
	second := &Account{ID: "user_second", Email: "second@example.com", Server: ConsoleBaseURL, AccessToken: "st_second", RefreshToken: "rt_second", ExpiresAt: time.Now().Add(time.Hour), IsActive: true}
	if err := store.Save(first); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if err := store.Save(second); err != nil {
		t.Fatalf("save second: %v", err)
	}

	selected := store.SelectForSession("ses_stable_conversation")
	if selected == nil {
		t.Fatal("expected an account")
	}
	for i := 0; i < 20; i++ {
		again := store.SelectForSession("ses_stable_conversation")
		if again == nil || again.ID != selected.ID {
			t.Fatalf("session selection is not sticky: got %v, want %s", again, selected.ID)
		}
	}

	if err := store.SetActive(selected.ID, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	other := store.SelectForSession("ses_stable_conversation")
	if other == nil || other.ID == selected.ID {
		t.Fatalf("expected the remaining active account, got %v", other)
	}

	if err := store.SetActive(other.ID, false); err != nil {
		t.Fatalf("deactivate remaining: %v", err)
	}
	if got := store.SelectForSession("ses_stable_conversation"); got != nil {
		t.Fatalf("expected nil when no accounts are active, got %+v", got)
	}
}

func TestAccountStoreMasksTokens(t *testing.T) {
	store, err := NewAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAccountStore: %v", err)
	}
	if err := store.Save(&Account{ID: "user_x", Email: "x@example.com", Server: ConsoleBaseURL, AccessToken: "st_1234567890abcdef", RefreshToken: "rt_1234567890abcdef", IsActive: true}); err != nil {
		t.Fatalf("save: %v", err)
	}

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 account, got %d", len(list))
	}
	if list[0].AccessToken == "st_1234567890abcdef" || !strings.Contains(list[0].AccessToken, "...") {
		t.Fatalf("expected masked access token, got %q", list[0].AccessToken)
	}
	if list[0].RefreshToken == "rt_1234567890abcdef" || !strings.Contains(list[0].RefreshToken, "...") {
		t.Fatalf("expected masked refresh token, got %q", list[0].RefreshToken)
	}
}

func TestIsFreeTierAndResponsesModels(t *testing.T) {
	free := []string{
		"big-pickle",
		"big-pickle-free",
		"deepseek-v4.1-flash-free",
		"muse-spark-1.3-contributor-free(xhigh)",
		"opencode/mimo-v2.6-flash-free",
	}
	for _, model := range free {
		if !IsFreeTierModel(model) {
			t.Errorf("expected %q to be free tier", model)
		}
	}

	paid := []string{"deepseek-v4-pro", "muse-spark-1.3", ""}
	for _, model := range paid {
		if IsFreeTierModel(model) {
			t.Errorf("expected %q to be paid", model)
		}
	}

	responses := []string{"gpt-5.6-luna", "grok-4.6", "muse-spark-1.3-contributor"}
	for _, model := range responses {
		if !IsResponsesModel(model) {
			t.Errorf("expected %q to use Responses", model)
		}
	}
	chat := []string{"deepseek-v4.1-flash", "claude-haiku-5-5", "big-pickle"}
	for _, model := range chat {
		if IsResponsesModel(model) {
			t.Errorf("expected %q to use chat completions", model)
		}
	}
}

type capturedRequest struct {
	path    string
	auth    string
	org     string
	request string
}

func captureUpstream(t *testing.T) (*httptest.Server, func() []capturedRequest) {
	t.Helper()

	var mu sync.Mutex
	var captured []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = append(captured, capturedRequest{
			path:    r.URL.Path,
			auth:    r.Header.Get("Authorization"),
			org:     r.Header.Get("x-opencode-org-id"),
			request: r.Header.Get("x-opencode-request"),
		})
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	return srv, func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedRequest{}, captured...)
	}
}

func newTestAccountStore(t *testing.T, accounts ...*Account) *AccountStore {
	t.Helper()
	store, err := NewAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAccountStore: %v", err)
	}
	for _, acc := range accounts {
		if err := store.Save(acc); err != nil {
			t.Fatalf("save account: %v", err)
		}
	}
	return store
}

func activeAccount() *Account {
	return &Account{
		ID:           "user_test",
		Email:        "test@example.com",
		Server:       ConsoleBaseURL,
		OrgID:        "org_test",
		OrgName:      "Personal",
		AccessToken:  "st_access",
		RefreshToken: "rt_refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		IsActive:     true,
	}
}

func TestExecuteChatAnonymousUsesPublicFreeLane(t *testing.T) {
	srv, requests := captureUpstream(t)
	client := NewClient(srv.URL, nil)

	resp, err := client.ExecuteChat(context.Background(), map[string]any{
		"model":    "big-pickle",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, "test-key", "ses_test_session")
	if err != nil {
		t.Fatalf("ExecuteChat: %v", err)
	}
	_ = resp.Body.Close()

	got := requests()
	if len(got) != 1 {
		t.Fatalf("expected 1 upstream request, got %d", len(got))
	}
	if got[0].path != "/inference/v1/chat/completions" {
		t.Errorf("path = %q, want /inference/v1/chat/completions", got[0].path)
	}
	if got[0].auth != "Bearer public" {
		t.Errorf("auth = %q, want Bearer public", got[0].auth)
	}
	if got[0].request == "" {
		t.Error("expected x-opencode-request on the anonymous lane")
	}
}

func TestExecuteChatOAuthAccountUsesGoLane(t *testing.T) {
	srv, requests := captureUpstream(t)
	client := NewClient(srv.URL, nil)
	client.SetAccountStore(newTestAccountStore(t, activeAccount()))

	resp, err := client.ExecuteChat(context.Background(), map[string]any{
		"model":    "deepseek-v4.1-flash",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, "test-key", "ses_test_session")
	if err != nil {
		t.Fatalf("ExecuteChat: %v", err)
	}
	_ = resp.Body.Close()

	got := requests()
	if len(got) != 1 {
		t.Fatalf("expected 1 upstream request, got %d", len(got))
	}
	if got[0].path != "/zen/go/v1/chat/completions" {
		t.Errorf("path = %q, want /zen/go/v1/chat/completions", got[0].path)
	}
	if got[0].auth != "Bearer st_access" {
		t.Errorf("auth = %q, want Bearer st_access", got[0].auth)
	}
	if got[0].org != "org_test" {
		t.Errorf("org = %q, want org_test", got[0].org)
	}
	if got[0].request != "" {
		t.Errorf("expected no x-opencode-request on the Go lane, got %q", got[0].request)
	}
}

func TestExecuteChatOAuthAccountFreeModelStaysAnonymous(t *testing.T) {
	srv, requests := captureUpstream(t)
	client := NewClient(srv.URL, nil)
	client.SetAccountStore(newTestAccountStore(t, activeAccount()))

	resp, err := client.ExecuteChat(context.Background(), map[string]any{
		"model":    "deepseek-v4.1-flash-free",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, "test-key", "ses_test_session")
	if err != nil {
		t.Fatalf("ExecuteChat: %v", err)
	}
	_ = resp.Body.Close()

	got := requests()
	if len(got) != 1 {
		t.Fatalf("expected 1 upstream request, got %d", len(got))
	}
	if got[0].path != "/inference/v1/chat/completions" {
		t.Errorf("path = %q, want /inference/v1/chat/completions", got[0].path)
	}
	if got[0].auth != "Bearer public" {
		t.Errorf("auth = %q, want Bearer public", got[0].auth)
	}
	if got[0].request == "" {
		t.Error("expected x-opencode-request for a free-tier model")
	}
}

func TestOAuthDeviceFlowAndRefresh(t *testing.T) {
	var mu sync.Mutex
	refreshCalls := 0
	console := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case deviceCodePath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "device-123",
				"user_code":                 "ABCD-EFGH",
				"verification_uri_complete": "/console/device?user_code=ABCD-EFGH&client_id=opencode-cli",
				"expires_in":                600,
				"interval":                  1,
			})
		case deviceTokenPath:
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["grant_type"] == "refresh_token" {
				mu.Lock()
				refreshCalls++
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "st_refreshed",
					"refresh_token": "rt_refreshed",
					"expires_in":    3600,
					"org_id":        "org_test",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "st_initial",
				"refresh_token": "rt_initial",
				"expires_in":    3600,
				"org_id":        "org_test",
			})
		case consoleUserPath:
			if r.Header.Get("Authorization") != "Bearer st_initial" && r.Header.Get("Authorization") != "Bearer st_refreshed" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":{"message":"unauthorized"}}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user_test", "email": "test@example.com"})
		case consoleOrgsPath:
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "org_test", "name": "Personal"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer console.Close()

	store := newTestAccountStore(t)
	client := NewClient("https://opencode.ai", nil)
	client.consoleBaseURL = console.URL
	client.SetAccountStore(store)

	auth, err := client.StartOAuth(context.Background())
	if err != nil {
		t.Fatalf("StartOAuth: %v", err)
	}
	if auth.UserCode != "ABCD-EFGH" || auth.DeviceCode != "device-123" {
		t.Fatalf("unexpected device authorization: %+v", auth)
	}
	if auth.VerificationURIComplete != console.URL+"/console/device?user_code=ABCD-EFGH&client_id=opencode-cli" {
		t.Errorf("verification URL = %q", auth.VerificationURIComplete)
	}

	acc, status, err := client.PollOAuth(context.Background(), auth.DeviceCode)
	if err != nil {
		t.Fatalf("PollOAuth: %v", err)
	}
	if status != "approved" || acc == nil {
		t.Fatalf("expected approved account, got status=%q account=%v", status, acc)
	}
	if acc.ID != "user_test" || acc.OrgID != "org_test" || acc.AccessToken != "st_initial" {
		t.Fatalf("unexpected account: %+v", acc)
	}

	// Expire the stored login and make sure ExecuteChat refreshes it first.
	if _, err := store.UpdateTokens(acc.ID, "st_initial", "rt_initial", time.Now().Add(-time.Minute), "", ""); err != nil {
		t.Fatalf("expire tokens: %v", err)
	}

	srv, requests := captureUpstream(t)
	client.baseURL = srv.URL
	resp, err := client.ExecuteChat(context.Background(), map[string]any{
		"model":    "deepseek-v4.1-flash",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}, "test-key", "ses_test_session")
	if err != nil {
		t.Fatalf("ExecuteChat: %v", err)
	}
	_ = resp.Body.Close()

	mu.Lock()
	gotRefreshCalls := refreshCalls
	mu.Unlock()
	if gotRefreshCalls != 1 {
		t.Errorf("expected 1 refresh call, got %d", gotRefreshCalls)
	}
	got := requests()
	if len(got) != 1 || got[0].auth != "Bearer st_refreshed" {
		t.Fatalf("expected refreshed token on the Go lane, got %+v", got)
	}

	fresh, ok := store.Get(acc.ID)
	if !ok || fresh.AccessToken != "st_refreshed" || fresh.RefreshToken != "rt_refreshed" {
		t.Errorf("store was not updated with refreshed tokens: %+v", fresh)
	}
}
