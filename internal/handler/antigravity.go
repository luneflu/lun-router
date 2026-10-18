package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lun-router/internal/antigravity"
	"lun-router/internal/model"
	"lun-router/internal/store"
)

type AntigravityHandler struct {
	client     *antigravity.Client
	modelStore *store.ModelStore
}

func NewAntigravityHandler(client *antigravity.Client, ms *store.ModelStore) *AntigravityHandler {
	return &AntigravityHandler{
		client:     client,
		modelStore: ms,
	}
}

// GET /api/providers/antigravity/accounts
func (h *AntigravityHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := h.client.Store().List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// POST /api/providers/antigravity/accounts
func (h *AntigravityHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email        string `json:"email"`
		ProjectID    string `json:"projectId"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Code         string `json:"code"`
		RedirectURI  string `json:"redirectUri"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Flow 1: OAuth code exchange
	if payload.Code != "" {
		code := strings.TrimSpace(payload.Code)
		// Extract code and redirectUri if user pasted a full callback URL
		if strings.Contains(code, "code=") {
			if u, err := url.Parse(code); err == nil && u.Host != "" {
				if payload.RedirectURI == "" {
					payload.RedirectURI = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.Path)
				}
				code = u.Query().Get("code")
			} else {
				parts := strings.Split(code, "code=")
				if len(parts) > 1 {
					sub := parts[1]
					if idx := strings.Index(sub, "&"); idx != -1 {
						code = sub[:idx]
					} else {
						code = sub
					}
				}
			}
		}

		acc, err := h.client.ExchangeCode(r.Context(), code, payload.RedirectURI)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(acc)
		return
	}

	// Flow 2: Manual credentials / 9router token paste
	if payload.RefreshToken == "" && payload.AccessToken == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Either authorization code or refresh/access token is required"})
		return
	}

	email := strings.TrimSpace(payload.Email)
	if email == "" {
		email = "manual@antigravity"
	}

	projectID := strings.TrimSpace(payload.ProjectID)
	if projectID == "" {
		projectID = "antigravity-project-" + time.Now().Format("060102")
	}

	acc := &antigravity.Account{
		Email:        email,
		ProjectID:    projectID,
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    time.Now().UTC().Add(1 * time.Hour),
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}

	// If access token is missing but refresh token exists, try refreshing immediately
	if acc.AccessToken == "" && acc.RefreshToken != "" {
		if err := h.client.RefreshToken(r.Context(), acc); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to validate refresh token: %v", err)})
			return
		}
	}

	if err := h.client.Store().Save(acc); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(acc)
}

// PUT /api/providers/antigravity/accounts/{id}
func (h *AntigravityHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"Missing account id"}`, http.StatusBadRequest)
		return
	}

	var payload struct {
		IsActive bool `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if err := h.client.Store().SetActive(id, payload.IsActive); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id, "isActive": payload.IsActive})
}

// DELETE /api/providers/antigravity/accounts/{id}
func (h *AntigravityHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"Missing account id"}`, http.StatusBadRequest)
		return
	}

	if err := h.client.Store().Delete(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": id})
}

// POST /api/providers/antigravity/accounts/{id}/refresh
func (h *AntigravityHandler) RefreshAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acc, ok := h.client.Store().Get(id)
	if !ok {
		http.Error(w, `{"error":"Account not found"}`, http.StatusNotFound)
		return
	}

	if err := h.client.RefreshToken(r.Context(), acc); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"expiresAt": acc.ExpiresAt,
	})
}

// GET /api/providers/antigravity/oauth/auth-url
func (h *AntigravityHandler) GetOAuthURL(w http.ResponseWriter, r *http.Request) {
	// The redirect URI stays on loopback: this client ID is Antigravity's own and
	// Google rejects any host that is not registered for it, such as a
	// reverse-proxied domain. The caller's value still wins when it names one.
	redirectURI := h.client.AuthRedirectURI(r.URL.Query().Get("redirect_uri"))
	state := r.URL.Query().Get("state")
	authURL := h.client.BuildAuthURL(redirectURI, state)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"authUrl":     authURL,
		"clientId":    antigravity.ClientID(),
		"redirectUri": redirectURI,
	})
}

// GET /api/providers/antigravity/models
func (h *AntigravityHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	var models []ModelItem
	catalog := h.client.GetCatalogModels()
	for _, m := range catalog {
		enabled := h.modelStore == nil || !h.modelStore.IsDisabled("antigravity", m)
		item := ModelItem{
			ID:      antigravityPrefix + m,
			Object:  "model",
			OwnedBy: "antigravity",
			Enabled: enabled,
		}
		model.EnrichModelItem(&item, "antigravity")
		models = append(models, item)
	}

	if h.modelStore != nil {
		for _, cm := range h.modelStore.ListCustomModels("antigravity") {
			item := ModelItem{
				ID:                  antigravityPrefix + cm.ID,
				Object:              "model",
				OwnedBy:             "antigravity",
				Enabled:             !h.modelStore.IsDisabled("antigravity", cm.ID),
				IsCustom:            true,
				Capabilities:        cm.Capabilities,
				ThinkingLevels:      cm.ThinkingLevels,
				ContextLength:       cm.ContextLength,
				MaxCompletionTokens: cm.MaxCompletionTokens,
			}
			if item.Capabilities == nil {
				model.EnrichModelItem(&item, "antigravity")
			}
			models = append(models, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "antigravity",
		"version":  "2.11.0",
		"models":   models,
	})
}

func (h *AntigravityHandler) AddCustomModel(w http.ResponseWriter, r *http.Request) {
	handleAddCustomModel(h.modelStore, "antigravity", w, r)
}

func (h *AntigravityHandler) DeleteCustomModel(w http.ResponseWriter, r *http.Request) {
	handleDeleteCustomModel(h.modelStore, "antigravity", w, r)
}

// POST /api/providers/antigravity/models/refresh (Sync latest models from Google Cloud Code)
func (h *AntigravityHandler) RefreshModels(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Antigravity client not initialized"})
		return
	}

	models, updated, err := h.client.FetchAvailableModels(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"count":   len(models),
			"updated": false,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"count":   len(models),
		"updated": updated,
	})
}

// PUT /api/providers/antigravity/models
func (h *AntigravityHandler) UpdateModels(w http.ResponseWriter, r *http.Request) {
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
			cleaned = append(cleaned, antigravity.CleanModelID(m))
		}
		err = h.modelStore.SetBatchDisabled("antigravity", cleaned, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled("antigravity", antigravity.CleanModelID(payload.Model), !payload.Enabled)
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

// GET /api/providers/antigravity/accounts/{id}/quota
func (h *AntigravityHandler) GetAccountQuota(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acc, ok := h.client.Store().Get(id)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Account not found"})
		return
	}

	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
	usage, err := h.client.GetAccountUsage(r.Context(), acc, force)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(usage)
}

// GET /api/providers/antigravity/quota
func (h *AntigravityHandler) GetAllQuotas(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
	usages, err := h.client.GetAllUsage(r.Context(), force)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"usages": usages,
	})
}
