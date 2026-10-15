package handler

import (
	"encoding/json"
	"net/http"

	"lun-router/internal/rtk"
)

type TokenSaverHandler struct {
	service *rtk.Service
}

func NewTokenSaverHandler(s *rtk.Service) *TokenSaverHandler {
	return &TokenSaverHandler{service: s}
}

// GET /api/tokensaver
func (h *TokenSaverHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rtk_enabled": false,
			"error":       "token saver service not available",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.GetStatus())
}

// PUT /api/tokensaver
func (h *TokenSaverHandler) Update(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "service not initialized"})
		return
	}

	var payload struct {
		RTKEnabled *bool `json:"rtk_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid JSON"})
		return
	}

	if payload.RTKEnabled != nil {
		if err := h.service.SetRTKEnabled(*payload.RTKEnabled); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.GetStatus())
}

// POST /api/tokensaver/test
func (h *TokenSaverHandler) TestCompress(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "service not initialized"})
		return
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid JSON"})
		return
	}

	res := h.service.TestCompress(payload.Text)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
