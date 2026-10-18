package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"lun-router/internal/kilocode"
	"lun-router/internal/model"
	"lun-router/internal/store"
)

type KiloCodeHandler struct {
	client     *kilocode.Client
	modelStore *store.ModelStore
}

func NewKiloCodeHandler(client *kilocode.Client, ms *store.ModelStore) *KiloCodeHandler {
	return &KiloCodeHandler{
		client:     client,
		modelStore: ms,
	}
}

// GET /api/providers/kilocode/accounts
func (h *KiloCodeHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := h.client.Store().List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// POST /api/providers/kilocode/accounts
func (h *KiloCodeHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email       string `json:"email"`
		OrgID       string `json:"orgId"`
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	token := strings.TrimSpace(payload.AccessToken)
	if token == "" {
		token = strings.TrimSpace(payload.Token)
	}

	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Access token is required"})
		return
	}

	acc, err := h.client.AddManualAccount(r.Context(), token, payload.Email, payload.OrgID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(acc)
}

// PUT /api/providers/kilocode/accounts/{id}
func (h *KiloCodeHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		IsActive *bool `json:"isActive"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	if payload.IsActive == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "isActive field is required"})
		return
	}

	if err := h.client.Store().SetActive(id, *payload.IsActive); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "isActive": *payload.IsActive})
}

// DELETE /api/providers/kilocode/accounts/{id}
func (h *KiloCodeHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.client.Store().Delete(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": id})
}

// POST /api/providers/kilocode/accounts/{id}/test
func (h *KiloCodeHandler) TestAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	latency, err := h.client.TestAccount(r.Context(), id)
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

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"latency_ms": latency,
	})
}

// RequestDeviceCode initiates device auth flow (GET /api/oauth/kilocode/device-code, POST /api/providers/kilocode/oauth/device-code)
func (h *KiloCodeHandler) RequestDeviceCode(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.RequestDeviceCode(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// Poll checks device auth approval (POST /api/oauth/kilocode/poll, POST /api/providers/kilocode/oauth/poll)
func (h *KiloCodeHandler) Poll(w http.ResponseWriter, r *http.Request) {
	var deviceCode string
	if r.Method == http.MethodGet {
		deviceCode = r.URL.Query().Get("deviceCode")
		if deviceCode == "" {
			deviceCode = r.URL.Query().Get("code")
		}
	} else {
		var payload struct {
			DeviceCode string `json:"deviceCode"`
			Code       string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			deviceCode = payload.DeviceCode
			if deviceCode == "" {
				deviceCode = payload.Code
			}
		}
	}

	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Missing device code"})
		return
	}

	acc, status, err := h.client.PollToken(r.Context(), deviceCode)
	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		statusCode := http.StatusBadRequest
		if status == "pending" {
			statusCode = http.StatusOK
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  status,
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if status == "approved" && acc != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "approved",
			"success": true,
			"account": acc,
		})
		return
	}

	// Status is pending or intermediate
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  status,
		"success": false,
	})
}

// GET /api/providers/kilocode/models
func (h *KiloCodeHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	var models []ModelItem
	catalog := h.client.GetCatalogModels()
	for _, m := range catalog {
		enabled := h.modelStore == nil || !h.modelStore.IsDisabled("kilocode", m)
		item := ModelItem{
			ID:      kilocodePrefix + m,
			Object:  "model",
			OwnedBy: "kilocode",
			Enabled: enabled,
		}
		model.EnrichModelItem(&item, "kilocode")
		models = append(models, item)
	}

	if h.modelStore != nil {
		for _, cm := range h.modelStore.ListCustomModels("kilocode") {
			item := ModelItem{
				ID:                  kilocodePrefix + cm.ID,
				Object:              "model",
				OwnedBy:             "kilocode",
				Enabled:             !h.modelStore.IsDisabled("kilocode", cm.ID),
				IsCustom:            true,
				Capabilities:        cm.Capabilities,
				ThinkingLevels:      cm.ThinkingLevels,
				ContextLength:       cm.ContextLength,
				MaxCompletionTokens: cm.MaxCompletionTokens,
			}
			if item.Capabilities == nil {
				model.EnrichModelItem(&item, "kilocode")
			}
			models = append(models, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "kilocode",
		"models":   models,
		"count":    len(models),
	})
}

func (h *KiloCodeHandler) AddCustomModel(w http.ResponseWriter, r *http.Request) {
	handleAddCustomModel(h.modelStore, "kilocode", w, r)
}

func (h *KiloCodeHandler) DeleteCustomModel(w http.ResponseWriter, r *http.Request) {
	handleDeleteCustomModel(h.modelStore, "kilocode", w, r)
}

// POST /api/providers/kilocode/models/refresh
func (h *KiloCodeHandler) RefreshModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.client.FetchModels(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": err.Error(),
			"count": len(models),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"count": len(models),
	})
}

// PUT /api/providers/kilocode/models
func (h *KiloCodeHandler) UpdateModels(w http.ResponseWriter, r *http.Request) {
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
			cleaned = append(cleaned, kilocode.CleanModelID(m))
		}
		err = h.modelStore.SetBatchDisabled("kilocode", cleaned, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled("kilocode", kilocode.CleanModelID(payload.Model), !payload.Enabled)
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

// GET /api/providers/kilo/free-models (9router parity)
func (h *KiloCodeHandler) GetFreeModels(w http.ResponseWriter, r *http.Request) {
	freeModels := h.client.GetFreeModels()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"models": freeModels,
	})
}
