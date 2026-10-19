package customprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStoreCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "customprov-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// 1. Add valid provider
	p, err := store.Add(Provider{
		Name:      "Groq",
		Prefix:    "groq",
		BaseURL:   "https://api.groq.com/openai/v1/",
		APIKey:    "gsk-1234567890abcdef",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if p.ID == "" {
		t.Errorf("expected generated ID")
	}
	if p.Prefix != "groq" {
		t.Errorf("expected prefix 'groq', got %q", p.Prefix)
	}
	if p.BaseURL != "https://api.groq.com/openai/v1" {
		t.Errorf("expected trailing slash stripped, got %q", p.BaseURL)
	}

	// 2. Reject duplicate prefix
	_, err = store.Add(Provider{
		Name:    "Groq 2",
		Prefix:  "groq",
		BaseURL: "https://api.groq.com/openai/v1",
	})
	if err == nil {
		t.Errorf("expected error for duplicate prefix")
	}

	// 3. Reject reserved prefix
	_, err = store.Add(Provider{
		Name:    "OpenCode Free Copy",
		Prefix:  "opencode",
		BaseURL: "https://example.com/v1",
	})
	if err == nil {
		t.Errorf("expected error for reserved prefix 'opencode'")
	}

	// 4. Reject invalid prefix format
	_, err = store.Add(Provider{
		Name:    "Invalid",
		Prefix:  "my prefix!",
		BaseURL: "https://example.com/v1",
	})
	if err == nil {
		t.Errorf("expected error for invalid prefix format")
	}

	// 5. Get and GetByPrefix
	got, ok := store.Get(p.ID)
	if !ok || got.Name != "Groq" {
		t.Errorf("Get failed or wrong provider")
	}

	gotByPrefix, ok := store.GetByPrefix("groq")
	if !ok || gotByPrefix.ID != p.ID {
		t.Errorf("GetByPrefix failed")
	}

	// 6. Update
	updated, err := store.Update(p.ID, func(target *Provider) error {
		target.Name = "Groq Fast"
		target.Prefix = "groq-fast"
		return nil
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "Groq Fast" || updated.Prefix != "groq-fast" {
		t.Errorf("Update fields mismatch")
	}

	// 7. SetModels
	err = store.SetModels(p.ID, []string{"llama-3.3-70b", "mixtral-8x7b", "llama-3.3-70b"})
	if err != nil {
		t.Fatalf("SetModels failed: %v", err)
	}
	gotModels, _ := store.Get(p.ID)
	if len(gotModels.Models) != 2 {
		t.Errorf("expected 2 unique models, got %d", len(gotModels.Models))
	}

	// 8. FindByModel
	foundByModel, ok := store.FindByModel("llama-3.3-70b")
	if !ok || foundByModel.ID != p.ID {
		t.Errorf("FindByModel failed")
	}

	// 9. Persistence
	store2, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("re-open store failed: %v", err)
	}
	if len(store2.List()) != 1 {
		t.Fatalf("expected 1 persisted provider, got %d", len(store2.List()))
	}

	// 10. Delete
	err = store2.Delete(p.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(store2.List()) != 0 {
		t.Errorf("expected 0 providers after delete")
	}
}

func TestClientFetchModelsAndExecuteChat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "llama-3.3-70b"},
					{"id": "deepseek-r1"},
				},
			})
		case "/v1/chat/completions":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["model"] != "llama-3.3-70b" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("model prefix was not stripped"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-123",
				"choices": []map[string]any{{"message": map[string]string{"content": "hello"}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewClient()
	p := Provider{
		ID:        "cp_test",
		Name:      "Mock OpenAI",
		Prefix:    "mock",
		BaseURL:   ts.URL + "/v1",
		APIKey:    "test-key",
		IsEnabled: true,
	}

	// FetchModels
	models, err := client.FetchModels(context.Background(), p)
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}
	if len(models) != 2 || models[0] != "deepseek-r1" || models[1] != "llama-3.3-70b" {
		t.Errorf("unexpected models: %v", models)
	}

	// ExecuteChat
	resp, err := client.ExecuteChat(context.Background(), p, map[string]any{
		"model": "mock/llama-3.3-70b",
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
	}, false)
	if err != nil {
		t.Fatalf("ExecuteChat failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ExecuteChat status code %d", resp.StatusCode)
	}
}
