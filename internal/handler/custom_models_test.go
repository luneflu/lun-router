package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"lun-router/internal/store"
)

func TestCustomModelsEndpoints(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-custom-models-handler-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ms, _ := store.NewModelStore(tempDir)
	compH := NewCompletionHandler(nil, ms, nil, nil, nil, nil, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/providers/opencode/models", compH.ProviderModels)
	mux.HandleFunc("POST /api/providers/opencode/models/custom", compH.AddCustomModel)
	mux.HandleFunc("DELETE /api/providers/opencode/models/custom/{id...}", compH.DeleteCustomModel)

	// 1. Add custom model
	body, _ := json.Marshal(map[string]any{
		"id":             "my-special-llm",
		"context_length": 65536,
		"vision":         true,
	})
	req := httptest.NewRequest("POST", "/api/providers/opencode/models/custom", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("AddCustomModel status %d: %s", w.Code, w.Body.String())
	}

	// 2. List provider models: should contain custom model with is_custom true
	req = httptest.NewRequest("GET", "/api/providers/opencode/models", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ProviderModels status %d", w.Code)
	}
	var provResp struct {
		Models []ModelItem `json:"models"`
	}
	_ = json.NewDecoder(w.Body).Decode(&provResp)

	found := false
	for _, m := range provResp.Models {
		if m.ID == "opencode/my-special-llm" {
			found = true
			if !m.IsCustom {
				t.Errorf("expected is_custom to be true")
			}
			if m.Capabilities == nil || !m.Capabilities.Vision {
				t.Errorf("expected vision capability to be true")
			}
			break
		}
	}
	if !found {
		t.Fatalf("custom model not found in provider models")
	}

	// 3. Delete custom model
	delReq := httptest.NewRequest("DELETE", "/api/providers/opencode/models/custom/my-special-llm", nil)
	delW := httptest.NewRecorder()
	mux.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("DeleteCustomModel status %d", delW.Code)
	}

	// 4. Verify gone
	req = httptest.NewRequest("GET", "/api/providers/opencode/models", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	_ = json.NewDecoder(w.Body).Decode(&provResp)

	for _, m := range provResp.Models {
		if m.ID == "opencode/my-special-llm" {
			t.Errorf("expected model to be deleted, but still present")
		}
	}
}
