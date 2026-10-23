package handler

import (
	"encoding/json"
	"net/http"
)

// GET /api/providers/opencode/accounts
func (h *CompletionHandler) OpenCodeListAccounts(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.AccountStore() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OpenCode account store not initialized")
		return
	}

	store := h.client.AccountStore()
	accounts := store.List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accounts":     accounts,
		"count":        len(accounts),
		"active_count": store.ActiveCount(),
	})
}

// POST /api/providers/opencode/accounts/oauth/start
func (h *CompletionHandler) OpenCodeOAuthStart(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OpenCode client not initialized")
		return
	}

	auth, err := h.client.StartOAuth(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                        true,
		"server":                    auth.Server,
		"device_code":               auth.DeviceCode,
		"user_code":                 auth.UserCode,
		"verification_uri":          auth.VerificationURI,
		"verification_uri_complete": auth.VerificationURIComplete,
		"expires_in":                auth.ExpiresIn,
		"interval":                  auth.Interval,
	})
}

// POST /api/providers/opencode/accounts/oauth/poll
func (h *CompletionHandler) OpenCodeOAuthPoll(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.AccountStore() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OpenCode account store not initialized")
		return
	}

	var payload struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	acc, status, err := h.client.PollOAuth(r.Context(), payload.DeviceCode)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     false,
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	if status == "approved" {
		h.invalidateOpenCodeCatalog()
	}
	if acc != nil {
		safe := *acc
		safe.AccessToken = ""
		safe.RefreshToken = ""
		acc = &safe
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      status == "approved",
		"status":  status,
		"account": acc,
	})
}

// PUT /api/providers/opencode/accounts/{id}
func (h *CompletionHandler) OpenCodeUpdateAccount(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.AccountStore() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OpenCode account store not initialized")
		return
	}

	var payload struct {
		IsActive *bool `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.IsActive == nil {
		writeJSONError(w, http.StatusBadRequest, "isActive field is required")
		return
	}

	if err := h.client.AccountStore().SetActive(r.PathValue("id"), *payload.IsActive); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	h.invalidateOpenCodeCatalog()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "isActive": *payload.IsActive})
}

// DELETE /api/providers/opencode/accounts/{id}
func (h *CompletionHandler) OpenCodeDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.AccountStore() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OpenCode account store not initialized")
		return
	}

	id := r.PathValue("id")
	if err := h.client.AccountStore().Delete(id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	h.invalidateOpenCodeCatalog()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": id})
}

// POST /api/providers/opencode/accounts/{id}/test
func (h *CompletionHandler) OpenCodeTestAccount(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.AccountStore() == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OpenCode account store not initialized")
		return
	}

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

func (h *CompletionHandler) invalidateOpenCodeCatalog() {
	if h.client != nil && h.client.ModelManager() != nil {
		h.client.ModelManager().InvalidateRaw()
	}
}
