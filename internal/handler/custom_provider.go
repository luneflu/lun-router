package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"lun-router/internal/customprovider"
	"lun-router/internal/model"
	"lun-router/internal/store"
)

type CustomProviderHandler struct {
	store      *customprovider.Store
	client     *customprovider.Client
	modelStore *store.ModelStore
}

func NewCustomProviderHandler(s *customprovider.Store, c *customprovider.Client, ms *store.ModelStore) *CustomProviderHandler {
	return &CustomProviderHandler{
		store:      s,
		client:     c,
		modelStore: ms,
	}
}

// GET /api/providers/custom - List all custom providers
func (h *CustomProviderHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"providers": []customprovider.ProviderSummary{}})
		return
	}

	items := h.store.List()
	summaries := make([]customprovider.ProviderSummary, len(items))
	for i, item := range items {
		summaries[i] = item.ToSummary()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"providers": summaries,
		"count":     len(summaries),
	})
}

// GET /api/providers/custom/{id} - Get single custom provider
func (h *CustomProviderHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, `{"error":"Custom provider store not configured"}`, http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	p, ok := h.store.Get(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Provider not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.ToSummary())
}

type CreateProviderPayload struct {
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey,omitempty"`
	IsEnabled *bool  `json:"isEnabled,omitempty"`
}

// POST /api/providers/custom - Create a new custom provider
func (h *CustomProviderHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, `{"error":"Custom provider store not configured"}`, http.StatusInternalServerError)
		return
	}

	var payload CreateProviderPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request JSON"})
		return
	}

	enabled := true
	if payload.IsEnabled != nil {
		enabled = *payload.IsEnabled
	}

	p, err := h.store.Add(customprovider.Provider{
		Name:      payload.Name,
		Prefix:    payload.Prefix,
		BaseURL:   payload.BaseURL,
		APIKey:    payload.APIKey,
		IsEnabled: enabled,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"provider": p.ToSummary(),
	})
}

type UpdateProviderPayload struct {
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	BaseURL   string `json:"baseUrl"`
	APIKey    *string `json:"apiKey,omitempty"`
	IsEnabled *bool   `json:"isEnabled,omitempty"`
}

// PUT /api/providers/custom/{id} - Update custom provider
func (h *CustomProviderHandler) Update(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, `{"error":"Custom provider store not configured"}`, http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	var payload UpdateProviderPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request JSON"})
		return
	}

	updated, err := h.store.Update(id, func(p *customprovider.Provider) error {
		if strings.TrimSpace(payload.Name) != "" {
			p.Name = payload.Name
		}
		if strings.TrimSpace(payload.Prefix) != "" {
			p.Prefix = payload.Prefix
		}
		if strings.TrimSpace(payload.BaseURL) != "" {
			p.BaseURL = payload.BaseURL
		}
		// If apiKey is provided, update it (empty string clears it)
		if payload.APIKey != nil {
			p.APIKey = strings.TrimSpace(*payload.APIKey)
		}
		if payload.IsEnabled != nil {
			p.IsEnabled = *payload.IsEnabled
		}
		return nil
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"provider": updated.ToSummary(),
	})
}

// DELETE /api/providers/custom/{id} - Delete custom provider
func (h *CustomProviderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, `{"error":"Custom provider store not configured"}`, http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	if err := h.store.Delete(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// POST /api/providers/custom/{id}/sync - Sync models from upstream provider
func (h *CustomProviderHandler) SyncModels(w http.ResponseWriter, r *http.Request) {
	if h.store == nil || h.client == nil {
		http.Error(w, `{"error":"Custom provider client not configured"}`, http.StatusInternalServerError)
		return
	}

	id := r.PathValue("id")
	p, ok := h.store.Get(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Provider not found"})
		return
	}

	models, err := h.client.FetchModels(r.Context(), p)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if err := h.store.SetModels(id, models); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"count":  len(models),
		"models": models,
	})
}

// GET /api/providers/custom/{id}/models - List models for this custom provider
func (h *CustomProviderHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, ok := h.store.Get(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Provider not found"})
		return
	}

	prefix := p.Prefix + "/"
	var models []ModelItem
	seen := make(map[string]bool)

	for _, m := range p.Models {
		seen[m] = true
		enabled := h.modelStore == nil || !h.modelStore.IsDisabled(p.Prefix, m)
		item := ModelItem{
			ID:      prefix + m,
			Object:  "model",
			OwnedBy: p.Prefix,
			Enabled: enabled,
		}
		model.EnrichModelItem(&item, p.Prefix)
		models = append(models, item)
	}

	if h.modelStore != nil {
		for _, cm := range h.modelStore.ListCustomModels(p.Prefix) {
			if !seen[cm.ID] {
				seen[cm.ID] = true
				item := ModelItem{
					ID:                  prefix + cm.ID,
					Object:              "model",
					OwnedBy:             p.Prefix,
					Enabled:             !h.modelStore.IsDisabled(p.Prefix, cm.ID),
					IsCustom:            true,
					Capabilities:        cm.Capabilities,
					ThinkingLevels:      cm.ThinkingLevels,
					ContextLength:       cm.ContextLength,
					MaxCompletionTokens: cm.MaxCompletionTokens,
				}
				if item.Capabilities == nil {
					model.EnrichModelItem(&item, p.Prefix)
				}
				models = append(models, item)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": p.Prefix,
		"name":     p.Name,
		"models":   models,
		"count":    len(models),
	})
}

// PUT /api/providers/custom/{id}/models - Enable / disable models
func (h *CustomProviderHandler) UpdateModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, ok := h.store.Get(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Provider not found"})
		return
	}

	if h.modelStore == nil {
		http.Error(w, `{"error":"Model store not configured"}`, http.StatusInternalServerError)
		return
	}

	var payload struct {
		Model   string   `json:"model"`
		Models  []string `json:"models"`
		Enabled bool     `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	var err error
	if len(payload.Models) > 0 {
		cleaned := make([]string, 0, len(payload.Models))
		for _, m := range payload.Models {
			cleaned = append(cleaned, customprovider.CleanModelID(p.Prefix, m))
		}
		err = h.modelStore.SetBatchDisabled(p.Prefix, cleaned, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled(p.Prefix, customprovider.CleanModelID(p.Prefix, payload.Model), !payload.Enabled)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Either model or models required"})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"enabled": payload.Enabled,
	})
}

// POST /api/providers/custom/{id}/models/custom - Add manual custom model
func (h *CustomProviderHandler) AddCustomModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, ok := h.store.Get(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Provider not found"})
		return
	}

	handleAddCustomModel(h.modelStore, p.Prefix, w, r)
}

// DELETE /api/providers/custom/{id}/models/custom - Delete manual custom model
func (h *CustomProviderHandler) DeleteCustomModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, ok := h.store.Get(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Provider not found"})
		return
	}

	handleDeleteCustomModel(h.modelStore, p.Prefix, w, r)
}
