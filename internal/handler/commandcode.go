package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"lun-router/internal/commandcode"
	"lun-router/internal/model"
	"lun-router/internal/store"
)

type CommandCodeHandler struct {
	client     *commandcode.Client
	modelStore *store.ModelStore
}

func NewCommandCodeHandler(client *commandcode.Client, ms *store.ModelStore) *CommandCodeHandler {
	return &CommandCodeHandler{client: client, modelStore: ms}
}

// GET /api/providers/commandcode/accounts
func (h *CommandCodeHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := h.client.Store().List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// POST /api/providers/commandcode/accounts
func (h *CommandCodeHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		APIKey string `json:"apiKey"`
		Token  string `json:"token"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	apiKey := strings.TrimSpace(payload.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(payload.Token)
	}
	if apiKey == "" {
		writeJSONError(w, http.StatusBadRequest, "API key is required")
		return
	}

	acc, err := h.client.AddManualAccount(r.Context(), apiKey, strings.TrimSpace(payload.Name))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(acc)
}

// PUT /api/providers/commandcode/accounts/{id}
func (h *CommandCodeHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		IsActive *bool `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.IsActive == nil {
		writeJSONError(w, http.StatusBadRequest, "isActive field is required")
		return
	}

	if err := h.client.Store().SetActive(id, *payload.IsActive); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "isActive": *payload.IsActive})
}

// DELETE /api/providers/commandcode/accounts/{id}
func (h *CommandCodeHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.client.Store().Delete(id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": id})
}

// POST /api/providers/commandcode/accounts/{id}/test
func (h *CommandCodeHandler) TestAccount(w http.ResponseWriter, r *http.Request) {
	latency, err := h.client.TestAccount(r.Context(), r.PathValue("id"))
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         false,
			"latency_ms": latency,
			"error":      err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "latency_ms": latency})
}

// GET /api/providers/commandcode/models
func (h *CommandCodeHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models := h.modelItems()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "commandcode",
		"models":   models,
		"count":    len(models),
	})
}

// POST /api/providers/commandcode/models/refresh
func (h *CommandCodeHandler) RefreshModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.client.FetchModels(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "count": len(models)})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "count": len(models)})
}

// PUT /api/providers/commandcode/models
func (h *CommandCodeHandler) UpdateModels(w http.ResponseWriter, r *http.Request) {
	if h.modelStore == nil {
		writeJSONError(w, http.StatusInternalServerError, "Model store not configured")
		return
	}

	var payload struct {
		Model   string   `json:"model"`
		Models  []string `json:"models"`
		Enabled bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	var err error
	if len(payload.Models) > 0 {
		cleaned := make([]string, 0, len(payload.Models))
		for _, m := range payload.Models {
			cleaned = append(cleaned, commandcode.CleanModelID(m))
		}
		err = h.modelStore.SetBatchDisabled("commandcode", cleaned, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled("commandcode", commandcode.CleanModelID(payload.Model), !payload.Enabled)
	} else {
		writeJSONError(w, http.StatusBadRequest, "Either model or models required")
		return
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "enabled": payload.Enabled})
}

func (h *CommandCodeHandler) AddCustomModel(w http.ResponseWriter, r *http.Request) {
	handleAddCustomModel(h.modelStore, "commandcode", w, r)
}

func (h *CommandCodeHandler) DeleteCustomModel(w http.ResponseWriter, r *http.Request) {
	handleDeleteCustomModel(h.modelStore, "commandcode", w, r)
}

func (h *CommandCodeHandler) modelItems() []ModelItem {
	var items []ModelItem
	for _, m := range h.client.GetCatalogModels() {
		item := ModelItem{
			ID:      commandcodePrefix + m.ID,
			Object:  "model",
			OwnedBy: "commandcode",
			Enabled: h.modelStore == nil || !h.modelStore.IsDisabled("commandcode", m.ID),
		}
		model.EnrichModelItem(&item, "commandcode")
		if m.ContextLength > 0 {
			item.ContextLength = m.ContextLength
		}
		if m.MaxOutput > 0 {
			item.MaxCompletionTokens = m.MaxOutput
		}
		items = append(items, item)
	}

	if h.modelStore != nil {
		for _, cm := range h.modelStore.ListCustomModels("commandcode") {
			item := ModelItem{
				ID:                  commandcodePrefix + cm.ID,
				Object:              "model",
				OwnedBy:             "commandcode",
				Enabled:             !h.modelStore.IsDisabled("commandcode", cm.ID),
				IsCustom:            true,
				Capabilities:        cm.Capabilities,
				ThinkingLevels:      cm.ThinkingLevels,
				ContextLength:       cm.ContextLength,
				MaxCompletionTokens: cm.MaxCompletionTokens,
			}
			if item.Capabilities == nil {
				model.EnrichModelItem(&item, "commandcode")
			}
			items = append(items, item)
		}
	}
	return items
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
