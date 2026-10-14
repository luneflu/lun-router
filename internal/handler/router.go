package handler

import (
	"net/http"

	"lun-router/internal/opencode"
	"lun-router/internal/store"
)

func NewRouter(ks *store.KeyStore, ms *store.ModelStore, client *opencode.Client) http.Handler {
	mux := http.NewServeMux()

	keyH := NewKeyHandler(ks)
	compH := NewCompletionHandler(ks, ms, client)

	// API Keys Management
	mux.HandleFunc("GET /api/health", keyH.Health)
	mux.HandleFunc("GET /api/keys", keyH.List)
	mux.HandleFunc("POST /api/keys", keyH.Create)
	mux.HandleFunc("GET /api/keys/{id}", keyH.Get)
	mux.HandleFunc("PUT /api/keys/{id}", keyH.Update)
	mux.HandleFunc("DELETE /api/keys/{id}", keyH.Delete)
	mux.HandleFunc("GET /api/keys/verify", keyH.Verify)

	// OpenCode Free API Routes (Issue #4781 bypass)
	mux.HandleFunc("GET /v1/models", compH.Models)
	mux.HandleFunc("GET /api/opencode/models", compH.Models)
	mux.HandleFunc("POST /v1/chat/completions", compH.ChatCompletions)
	mux.HandleFunc("POST /api/chat/completions", compH.ChatCompletions)

	// Provider Model Management (Enable / Disable)
	mux.HandleFunc("GET /api/providers/opencode/models", compH.ProviderModels)
	mux.HandleFunc("PUT /api/providers/opencode/models", compH.UpdateProviderModels)
	mux.HandleFunc("POST /api/providers/opencode/version/refresh", compH.RefreshVersion)
	mux.HandleFunc("POST /api/models/test", compH.TestModel)

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
