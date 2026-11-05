package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"lun-router/internal/customprovider"
	"lun-router/internal/store"
)

func setupTestCustomProviderEnvironment(t *testing.T) (*customprovider.Store, *store.ModelStore, *store.KeyStore, *httptest.Server, http.Handler) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "handler-custom-prov-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	ks, err := store.NewKeyStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ks.Create("test-user", "")
	if err != nil {
		t.Fatal(err)
	}

	ms, err := store.NewModelStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	cpStore, err := customprovider.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Mock upstream OpenAI server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
			return
		}

		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "llama-3.3-70b"},
					{
						"id":             "deepseek-v3",
						"name":           "DeepSeek V3",
						"context_window": 65536,
						"max_tokens":     8192,
						"capabilities": map[string]any{
							"reasoning": true,
							"vision":    true,
						},
					},
				},
			})
		case "/v1/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			modelName, _ := body["model"].(string)
			// Ensure prefix was stripped
			if modelName != "llama-3.3-70b" && modelName != "deepseek-v3" && modelName != "custom-added" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "unexpected model: " + modelName})
				return
			}

			if stream, _ := body["stream"].(bool); stream {
				w.Header().Set("Content-Type", "text/event-stream")
				flusher, _ := w.(http.Flusher)
				_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello from custom!\"}}]}\n\n")
				if flusher != nil {
					flusher.Flush()
				}
				_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
				if flusher != nil {
					flusher.Flush()
				}
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-test",
				"object":  "chat.completion",
				"created": 12345678,
				"model":   modelName,
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]string{
							"role":    "assistant",
							"content": "Custom provider response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int{
					"prompt_tokens":     10,
					"completion_tokens": 5,
					"total_tokens":      15,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstreamServer.Close)

	cpClient := customprovider.NewClient()
	router := NewRouterWithCustomProviders(ks, ms, nil, nil, nil, nil, nil, cpStore, cpClient, nil, nil, nil, nil, nil)

	// Save active test key into context/headers for requests
	_ = key
	return cpStore, ms, ks, upstreamServer, router
}

func TestCustomProviderAPI_Lifecycle(t *testing.T) {
	cpStore, _, ks, upstream, router := setupTestCustomProviderEnvironment(t)
	keys := ks.List()

	// 1. List initially empty
	req := httptest.NewRequest(http.MethodGet, "/api/providers/custom", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("List status: %d", rec.Code)
	}
	var listResp struct {
		Providers []customprovider.ProviderSummary `json:"providers"`
		Count     int                              `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatal(err)
	}
	if listResp.Count != 0 {
		t.Fatalf("expected 0 providers, got %d", listResp.Count)
	}

	// 2. Create Custom Provider
	createBody, _ := json.Marshal(map[string]any{
		"name":      "My Groq",
		"prefix":    "groq",
		"baseUrl":   upstream.URL + "/v1",
		"apiKey":    "test-secret",
		"isEnabled": true,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/providers/custom", bytes.NewReader(createBody))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create status: %d, body: %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		OK       bool                           `json:"ok"`
		Provider customprovider.ProviderSummary `json:"provider"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&createResp)
	provID := createResp.Provider.ID
	if provID == "" || createResp.Provider.Prefix != "groq" || !createResp.Provider.HasAPIKey {
		t.Fatalf("unexpected created provider: %+v", createResp.Provider)
	}

	// 3. Sync Models
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/providers/custom/%s/sync", provID), nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Sync status: %d, body: %s", rec.Code, rec.Body.String())
	}
	var syncResp struct {
		OK     bool     `json:"ok"`
		Count  int      `json:"count"`
		Models []string `json:"models"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&syncResp)
	if !syncResp.OK || syncResp.Count != 2 {
		t.Fatalf("unexpected sync models resp: %+v", syncResp)
	}

	// 4. List Models
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/providers/custom/%s/models", provID), nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListModels status: %d, body: %s", rec.Code, rec.Body.String())
	}
	var modelsResp struct {
		Provider string      `json:"provider"`
		Models   []ModelItem `json:"models"`
		Count    int         `json:"count"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&modelsResp)
	if modelsResp.Count != 2 || modelsResp.Provider != "groq" {
		t.Fatalf("unexpected ListModels: %+v", modelsResp)
	}
	// Discovered metadata must reach the dashboard payload, not just /v1/models.
	var dashDeepSeek *ModelItem
	for i := range modelsResp.Models {
		if modelsResp.Models[i].ID == "groq/deepseek-v3" {
			dashDeepSeek = &modelsResp.Models[i]
		}
	}
	if dashDeepSeek == nil {
		t.Fatalf("deepseek-v3 missing from dashboard models: %+v", modelsResp.Models)
	}
	if dashDeepSeek.Name != "DeepSeek V3" {
		t.Errorf("dashboard name: got %q, want DeepSeek V3", dashDeepSeek.Name)
	}
	if dashDeepSeek.ContextLength != 65536 || dashDeepSeek.MaxCompletionTokens != 8192 {
		t.Errorf("dashboard limits: got ctx=%d max=%d", dashDeepSeek.ContextLength, dashDeepSeek.MaxCompletionTokens)
	}
	if len(dashDeepSeek.ThinkingLevels) == 0 {
		t.Errorf("dashboard thinking_levels empty for reasoning model")
	}
	if dashDeepSeek.Capabilities == nil || !dashDeepSeek.Capabilities.Reasoning || !dashDeepSeek.Capabilities.Vision {
		t.Errorf("dashboard capabilities: %+v", dashDeepSeek.Capabilities)
	}

	// 5. Add Custom Model
	customModelBody, _ := json.Marshal(map[string]any{
		"id":             "custom-added",
		"context_length": 65536,
		"tools":          true,
	})
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/providers/custom/%s/models/custom", provID), bytes.NewReader(customModelBody))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("AddCustomModel status: %d, body: %s", rec.Code, rec.Body.String())
	}

	// 6. Check /v1/models reflects custom provider models with prefix
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/models status: %d", rec.Code)
	}
	var v1Models struct {
		Data []ModelItem `json:"data"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&v1Models)
	foundGroqLlama := false
	foundGroqCustom := false
	foundDeepSeekThinking := false
	for _, m := range v1Models.Data {
		if m.ID == "groq/llama-3.3-70b" {
			foundGroqLlama = true
		}
		if m.ID == "groq/custom-added" {
			foundGroqCustom = true
		}
		if m.ID == "groq/deepseek-v3" && len(m.ThinkingLevels) > 0 {
			foundDeepSeekThinking = true
		}
	}
	if !foundGroqLlama || !foundGroqCustom {
		t.Errorf("missing groq models in /v1/models. foundLlama=%v, foundCustom=%v", foundGroqLlama, foundGroqCustom)
	}
	if !foundDeepSeekThinking {
		t.Errorf("/v1/models missing thinking_levels for groq/deepseek-v3")
	}

	// 7. Chat completions (non-stream)
	chatBody, _ := json.Marshal(map[string]any{
		"model": "groq/llama-3.3-70b",
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(chatBody))
	req.Header.Set("Authorization", "Bearer "+keys[0].Key)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Chat completions status: %d, body: %s", rec.Code, rec.Body.String())
	}

	// 8. Chat completions (streaming)
	streamChatBody, _ := json.Marshal(map[string]any{
		"model":  "groq/llama-3.3-70b",
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": "hello stream"},
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(streamChatBody))
	req.Header.Set("Authorization", "Bearer "+keys[0].Key)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Stream chat completions status: %d, body: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("Hello from custom!")) {
		t.Errorf("Stream response did not contain expected text: %s", rec.Body.String())
	}

	// 9. Model Ping / Test
	testPingBody, _ := json.Marshal(map[string]any{
		"model":  "groq/llama-3.3-70b",
		"prompt": "ping",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/models/test", bytes.NewReader(testPingBody))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Ping status: %d, body: %s", rec.Code, rec.Body.String())
	}
	var pingResp ModelTestResult
	_ = json.NewDecoder(rec.Body).Decode(&pingResp)
	if !pingResp.OK {
		t.Fatalf("Model ping failed: %v", pingResp.Error)
	}

	// 10. Disable provider and test rejection
	_ = cpStore.SetEnabled(provID, false)
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(chatBody))
	req.Header.Set("Authorization", "Bearer "+keys[0].Key)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for disabled provider, got %d", rec.Code)
	}

	// 11. Delete Provider
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/providers/custom/%s", provID), nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Delete status: %d", rec.Code)
	}
}

func TestCustomProviderStaleModelsAutoRefresh(t *testing.T) {
	cpStore, _, _, upstream, router := setupTestCustomProviderEnvironment(t)

	// Simulate a provider saved before metadata capture: bare model IDs only.
	p, err := cpStore.Add(customprovider.Provider{
		Name:      "Legacy Groq",
		Prefix:    "legacygroq",
		BaseURL:   upstream.URL + "/v1",
		APIKey:    "test-secret",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if _, err := cpStore.Update(p.ID, func(pp *customprovider.Provider) error {
		pp.Models = []customprovider.Model{{ID: "llama-3.3-70b"}, {ID: "deepseek-v3"}}
		pp.ModelsSyncedAt = time.Time{}
		return nil
	}); err != nil {
		t.Fatalf("seed legacy models failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/providers/custom/%s/models", p.ID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListModels status: %d, body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Models []ModelItem `json:"models"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	var deepSeek *ModelItem
	for i := range resp.Models {
		if resp.Models[i].ID == "legacygroq/deepseek-v3" {
			deepSeek = &resp.Models[i]
		}
	}
	if deepSeek == nil {
		t.Fatalf("deepseek-v3 missing after auto refresh: %+v", resp.Models)
	}
	if deepSeek.Name != "DeepSeek V3" {
		t.Errorf("name: got %q, want DeepSeek V3", deepSeek.Name)
	}
	if len(deepSeek.ThinkingLevels) == 0 {
		t.Errorf("thinking_levels empty after auto refresh: %+v", deepSeek)
	}

	// Metadata must be persisted, so the next read needs no upstream call.
	stored, _ := cpStore.Get(p.ID)
	if stored.NeedsModelsRefresh() {
		t.Errorf("catalog still marked stale after refresh")
	}
}
