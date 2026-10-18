package kilocode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAccountStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kilo-store-test-*")
	if err != nil {
		t.Fatalf("tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewAccountStore(tempDir)
	if err != nil {
		t.Fatalf("NewAccountStore: %v", err)
	}

	acc := &Account{
		Email:       "test@kilocode.ai",
		OrgID:       "org_123",
		AccessToken: "test-access-token-1234567890",
		IsActive:    true,
	}

	if err := store.Save(acc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if store.Count() != 1 {
		t.Errorf("expected 1 account, got %d", store.Count())
	}
	if store.ActiveCount() != 1 {
		t.Errorf("expected 1 active account, got %d", store.ActiveCount())
	}

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 listed, got %d", len(list))
	}
	if list[0].AccessToken == "test-access-token-1234567890" {
		t.Errorf("expected masked token in List(), got unmasked")
	}

	active, ok := store.GetActive()
	if !ok || active.Email != "test@kilocode.ai" {
		t.Errorf("GetActive failed")
	}

	// Toggle active
	if err := store.SetActive(acc.ID, false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if store.ActiveCount() != 0 {
		t.Errorf("expected 0 active accounts, got %d", store.ActiveCount())
	}

	// Delete
	if err := store.Delete(acc.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Count() != 0 {
		t.Errorf("expected 0 accounts after delete, got %d", store.Count())
	}
}

func TestDeviceCodeAndPollFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kilo-client-test-*")
	if err != nil {
		t.Fatalf("tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, _ := NewAccountStore(tempDir)
	client := NewClient(store, tempDir)

	// Mock server for Kilo API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/device-auth/codes":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":            "KC-1234-TEST",
				"verificationUrl": "https://kilo.ai/auth/verify?code=KC-1234-TEST",
				"expiresIn":       300,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/device-auth/codes/pending-code":
			w.WriteHeader(http.StatusAccepted) // 202
		case r.Method == http.MethodGet && r.URL.Path == "/api/device-auth/codes/approved-code":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":    "approved",
				"token":     "kilo_sec_token_999",
				"userEmail": "dev@kilo.ai",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/profile":
			if r.Header.Get("Authorization") != "Bearer kilo_sec_token_999" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"email": "dev@kilo.ai",
				"organizations": []map[string]string{
					{"id": "org_kilo_abc", "name": "Kilo Devs"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	// Direct tests on helper functions
	if !HasKilocodePrefix("kilocode/deepseek/deepseek-chat") {
		t.Errorf("expected HasKilocodePrefix to be true")
	}
	if !HasKilocodePrefix("kilo/deepseek/deepseek-chat") {
		t.Errorf("expected HasKilocodePrefix to be true")
	}
	if !HasKilocodePrefix("kc/deepseek/deepseek-chat") {
		t.Errorf("expected HasKilocodePrefix to be true")
	}
	if CleanModelID("kilocode/deepseek/deepseek-chat") != "deepseek/deepseek-chat" {
		t.Errorf("expected cleaned model ID, got %s", CleanModelID("kilocode/deepseek/deepseek-chat"))
	}
	if CleanModelID("kc/gpt-4.1") != "gpt-4.1" {
		t.Errorf("expected gpt-4.1, got %s", CleanModelID("kc/gpt-4.1"))
	}

	// Verify FallbackModels
	if len(client.GetCatalogModels()) == 0 {
		t.Errorf("expected catalog models to be populated with fallbacks")
	}
}
