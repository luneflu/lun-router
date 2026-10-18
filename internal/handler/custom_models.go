package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"lun-router/internal/model"
	"lun-router/internal/store"
)

type AddCustomModelRequest struct {
	ID                  string                   `json:"id"`
	ContextLength       int                      `json:"context_length,omitempty"`
	MaxCompletionTokens int                      `json:"max_completion_tokens,omitempty"`
	Vision              bool                     `json:"vision,omitempty"`
	Reasoning           bool                     `json:"reasoning,omitempty"`
	Tools               bool                     `json:"tools,omitempty"`
	ThinkingLevels      []string                 `json:"thinking_levels,omitempty"`
}

func handleAddCustomModel(ms *store.ModelStore, provider string, w http.ResponseWriter, r *http.Request) {
	if ms == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model store unavailable"})
		return
	}

	var req AddCustomModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request JSON"})
		return
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model id is required"})
		return
	}

	// Strip provider prefix if user passed it
	prefix := provider + "/"
	if strings.HasPrefix(id, prefix) {
		id = strings.TrimPrefix(id, prefix)
	}

	var caps *model.ModelCapabilities
	if req.Vision || req.Reasoning || req.Tools || req.ContextLength > 0 || req.MaxCompletionTokens > 0 {
		caps = &model.ModelCapabilities{
			Vision:         req.Vision,
			Reasoning:      req.Reasoning,
			Tools:          req.Tools,
			ContextWindow:  req.ContextLength,
			MaxOutput:      req.MaxCompletionTokens,
			ThinkingLevels: req.ThinkingLevels,
		}
	}

	cm := store.CustomModel{
		ID:                  id,
		Provider:            provider,
		ContextLength:       req.ContextLength,
		MaxCompletionTokens: req.MaxCompletionTokens,
		Capabilities:        caps,
		ThinkingLevels:      req.ThinkingLevels,
		CreatedAt:           time.Now(),
	}

	if err := ms.AddCustomModel(provider, cm); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	item := ModelItem{
		ID:                  prefix + id,
		Object:              "model",
		OwnedBy:             provider,
		Enabled:             true,
		IsCustom:            true,
		Capabilities:        cm.Capabilities,
		ThinkingLevels:      cm.ThinkingLevels,
		ContextLength:       cm.ContextLength,
		MaxCompletionTokens: cm.MaxCompletionTokens,
	}
	if item.Capabilities == nil {
		model.EnrichModelItem(&item, provider)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"model":   item,
	})
}

func handleDeleteCustomModel(ms *store.ModelStore, provider string, w http.ResponseWriter, r *http.Request) {
	if ms == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model store unavailable"})
		return
	}

	// 1. Check path value
	id := r.PathValue("id")
	// 2. Fall back to query param
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	if id == "" {
		id = r.URL.Query().Get("model")
	}
	// 3. Fall back to body
	if id == "" && r.Body != nil {
		var bodyObj struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&bodyObj)
		if bodyObj.ID != "" {
			id = bodyObj.ID
		} else {
			id = bodyObj.Model
		}
	}

	id = strings.TrimSpace(id)
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model id is required"})
		return
	}

	prefix := provider + "/"
	if strings.HasPrefix(id, prefix) {
		id = strings.TrimPrefix(id, prefix)
	}

	if err := ms.DeleteCustomModel(provider, id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
	})
}
