package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"lun-router/internal/caveman"
)

type CavemanHandler struct {
	service *caveman.Service
}

func NewCavemanHandler(service *caveman.Service) *CavemanHandler {
	return &CavemanHandler{service: service}
}

// GET /api/caveman
func (h *CavemanHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Caveman service not initialized"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.GetStatus())
}

// PUT /api/caveman
func (h *CavemanHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Caveman service not initialized"})
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

// POST /api/caveman/sync (Triggers sync with https://github.com/JuliusBrussee/caveman)
func (h *CavemanHandler) SyncUpstream(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Caveman service not initialized"})
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

// POST /api/caveman/test
func (h *CavemanHandler) TestPrompt(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Caveman service not initialized"})
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
			req.Model = "gemini-3.8-flash"
		} else {
			req.Model = "big-pickle"
		}
	}
	if req.UserPrompt == "" {
		req.UserPrompt = "Why is my React component re-rendering on every keystroke?"
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

	// Estimate savings
	originalTokens := len(strings.Fields(req.UserPrompt)) * 4
	estResponseSavings := "54% - 75% fewer output tokens"

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"applied":           applied,
		"provider":          req.Provider,
		"model":             req.Model,
		"mode":              req.Mode,
		"injected_prompt":   injectedPrompt,
		"messages":          injectedMessages,
		"prompt_tokens_est": originalTokens,
		"estimated_savings": estResponseSavings,
		"timestamp":         time.Now().Format(time.RFC3339),
	})
}
