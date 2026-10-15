package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"lun-router/internal/auth"
)

type AuthHandler struct {
	svc *auth.Service
}

func NewAuthHandler(svc *auth.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type AuthStatusResponse struct {
	AuthEnabled   bool `json:"auth_enabled"`
	SetupRequired bool `json:"setup_required"`
	Authenticated bool `json:"authenticated"`
}

type PasswordRequest struct {
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ExtractSessionToken extracts session token from cookie, Authorization header, or X-Auth-Token
func ExtractSessionToken(r *http.Request) string {
	if cookie, err := r.Cookie("lun_session"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if !strings.HasPrefix(token, "lun-") {
			return token
		}
	}
	if xToken := r.Header.Get("X-Auth-Token"); xToken != "" {
		return strings.TrimSpace(xToken)
	}
	return ""
}

// GET /api/auth/status
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.svc == nil || !h.svc.IsAuthEnabled() {
		json.NewEncoder(w).Encode(AuthStatusResponse{
			AuthEnabled:   false,
			SetupRequired: true,
			Authenticated: true,
		})
		return
	}

	token := ExtractSessionToken(r)
	authenticated := h.svc.ValidateSession(token)

	json.NewEncoder(w).Encode(AuthStatusResponse{
		AuthEnabled:   true,
		SetupRequired: false,
		Authenticated: authenticated,
	})
}

// POST /api/auth/setup
func (h *AuthHandler) Setup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.svc == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Auth service unavailable"})
		return
	}

	if h.svc.IsAuthEnabled() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Password already configured"})
		return
	}

	var req PasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if err := h.svc.SetupPassword(req.Password); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	token, err := h.svc.CreateSession()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create session"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "lun_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600, // 30 days
	})

	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"token":   token,
	})
}

// POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.svc == nil || !h.svc.IsAuthEnabled() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Auth not configured. Please setup password first."})
		return
	}

	var req PasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if !h.svc.VerifyPassword(req.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid password"})
		return
	}

	token, err := h.svc.CreateSession()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create session"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "lun_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})

	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"token":   token,
	})
}

// POST /api/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.svc != nil {
		token := ExtractSessionToken(r)
		if token != "" {
			h.svc.RevokeSession(token)
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "lun_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// POST /api/auth/change-password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.svc == nil || !h.svc.IsAuthEnabled() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Auth not enabled"})
		return
	}

	token := ExtractSessionToken(r)
	if !h.svc.ValidateSession(token) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if err := h.svc.ChangePassword(req.CurrentPassword, req.NewPassword); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
