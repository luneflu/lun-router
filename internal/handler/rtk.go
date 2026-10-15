package handler

import (
	"encoding/json"
	"net/http"

	"lun-router/internal/rtk"
)

type RTKHandler struct {
	service *rtk.Service
}

func NewRTKHandler(s *rtk.Service) *RTKHandler {
	return &RTKHandler{service: s}
}

// GET /api/rtk
func (h *RTKHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rtk_enabled": false,
			"error":       "rtk service not available",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.GetStatus())
}

// PUT /api/rtk
func (h *RTKHandler) Update(w http.ResponseWriter, r *http.Request) {
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

// POST /api/rtk/test
func (h *RTKHandler) TestCompress(w http.ResponseWriter, r *http.Request) {
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
