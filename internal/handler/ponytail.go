package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"lun-router/internal/ponytail"
)

type PonytailHandler struct {
	service *ponytail.Service
}

func NewPonytailHandler(service *ponytail.Service) *PonytailHandler {
	return &PonytailHandler{service: service}
}

// GET /api/ponytail
func (h *PonytailHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Ponytail service not initialized"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.GetStatus())
}

// PUT /api/ponytail
func (h *PonytailHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Ponytail service not initialized"})
		return
	}

	var req struct {
		Enabled      *bool   `json:"enabled"`
		Mode         *string `json:"mode"`
		AutoUpdate   *bool   `json:"auto_update"`
		CustomPrompt *string `json:"custom_prompt"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	st, err := h.service.UpdateConfig(req.Enabled, req.Mode, req.AutoUpdate, req.CustomPrompt)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// POST /api/ponytail/sync (Triggers sync with https://github.com/DietrichGebert/ponytail)
func (h *PonytailHandler) SyncUpstream(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Ponytail service not initialized"})
		return
	}

	ctx := r.Context()
	updated, err := h.service.SyncUpstream(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"updated": false,
			"error":   err.Error(),
			"status":  h.service.GetStatus(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"updated": updated,
		"status":  h.service.GetStatus(),
	})
}

// POST /api/ponytail/test
func (h *PonytailHandler) TestPrompt(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Ponytail service not initialized"})
		return
	}

	var req struct {
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		Mode        string `json:"mode"`
		UserPrompt  string `json:"user_prompt"`
		SystemPrior string `json:"system_prior"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Provider == "" {
		req.Provider = "opencode"
	}
	if req.Model == "" {
		if req.Provider == "antigravity" {
			req.Model = "gemini-3.8-flash-high"
		} else {
			req.Model = "muse-spark-1.3-contributor-free"
		}
	}
	if req.UserPrompt == "" {
		req.UserPrompt = "Build me a rate limiter in Go with token bucket"
	}

	var messages []any
	if req.SystemPrior != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": req.SystemPrior,
		})
	}
	messages = append(messages, map[string]any{
		"role":    "user",
		"content": req.UserPrompt,
	})

	body := map[string]any{
		"model":    req.Model,
		"messages": messages,
	}

	applied := h.service.Apply(body, "on", req.Mode)
	injectedMessages := body["messages"].([]any)

	var injectedPrompt string
	if len(injectedMessages) > 0 {
		if first, ok := injectedMessages[0].(map[string]any); ok {
			if content, ok := first["content"].(string); ok {
				injectedPrompt = content
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"applied":         applied,
		"provider":        req.Provider,
		"model":           req.Model,
		"mode":            req.Mode,
		"injected_prompt": injectedPrompt,
		"messages":        injectedMessages,
		"timestamp":       time.Now().Format(time.RFC3339),
	})
}
