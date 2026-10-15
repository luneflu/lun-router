package handler

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"lun-router/internal/antigravity"
	"lun-router/internal/auth"
	"lun-router/internal/caveman"
	"lun-router/internal/compactor"
	"lun-router/internal/opencode"
	"lun-router/internal/ponytail"
	"lun-router/internal/store"
)

func NewRouter(ks *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, compactorSvc *compactor.Service, cavemanSvc *caveman.Service, ponytailSvc *ponytail.Service, authSvc *auth.Service, staticFS ...fs.FS) http.Handler {
	mux := http.NewServeMux()

	keyH := NewKeyHandler(ks)
	compH := NewCompletionHandler(ks, ms, client, agClient, compactorSvc, cavemanSvc, ponytailSvc)
	compactH := NewCompactorHandler(compactorSvc)
	caveH := NewCavemanHandler(cavemanSvc)
	ponyH := NewPonytailHandler(ponytailSvc)

	// Auth Management
	if authSvc != nil {
		authH := NewAuthHandler(authSvc)
		mux.HandleFunc("GET /api/auth/status", authH.Status)
		mux.HandleFunc("POST /api/auth/setup", authH.Setup)
		mux.HandleFunc("POST /api/auth/login", authH.Login)
		mux.HandleFunc("POST /api/auth/logout", authH.Logout)
		mux.HandleFunc("POST /api/auth/change-password", authH.ChangePassword)
	}

	// API Keys Management
	mux.HandleFunc("GET /api/health", keyH.Health)
	mux.HandleFunc("GET /api/keys", keyH.List)
	mux.HandleFunc("POST /api/keys", keyH.Create)
	mux.HandleFunc("GET /api/keys/{id}", keyH.Get)
	mux.HandleFunc("PUT /api/keys/{id}", keyH.Update)
	mux.HandleFunc("DELETE /api/keys/{id}", keyH.Delete)
	mux.HandleFunc("GET /api/keys/verify", keyH.Verify)

	// Compactor Routes (tool output compression)
	mux.HandleFunc("GET /api/compactor", compactH.GetStatus)
	mux.HandleFunc("PUT /api/compactor", compactH.Update)
	mux.HandleFunc("POST /api/compactor/test", compactH.TestCompress)

	// Caveman Routes (Prompt & Voice Optimization)
	mux.HandleFunc("GET /api/caveman", caveH.GetStatus)
	mux.HandleFunc("PUT /api/caveman", caveH.UpdateConfig)
	mux.HandleFunc("POST /api/caveman/sync", caveH.SyncUpstream)
	mux.HandleFunc("POST /api/caveman/test", caveH.TestPrompt)

	// Ponytail Routes (Lazy Senior Developer & Anti-Bloat)
	mux.HandleFunc("GET /api/ponytail", ponyH.GetStatus)
	mux.HandleFunc("PUT /api/ponytail", ponyH.UpdateConfig)
	mux.HandleFunc("POST /api/ponytail/sync", ponyH.SyncUpstream)
	mux.HandleFunc("POST /api/ponytail/test", ponyH.TestPrompt)

	// OpenCode Free API Routes (Issue #4781 bypass)
	mux.HandleFunc("GET /v1/models", compH.Models)
	mux.HandleFunc("GET /api/opencode/models", compH.Models)
	mux.HandleFunc("POST /v1/chat/completions", compH.ChatCompletions)
	mux.HandleFunc("POST /api/chat/completions", compH.ChatCompletions)

	// Provider Model Management (Enable / Disable)
	mux.HandleFunc("GET /api/providers/opencode/models", compH.ProviderModels)
	mux.HandleFunc("PUT /api/providers/opencode/models", compH.UpdateProviderModels)
	mux.HandleFunc("POST /api/providers/opencode/models/refresh", compH.RefreshOpenCodeModels)
	mux.HandleFunc("POST /api/providers/opencode/version/refresh", compH.RefreshVersion)
	mux.HandleFunc("POST /api/models/test", compH.TestModel)

	// Antigravity Provider Routes (9router parity)
	if agClient != nil {
		agH := NewAntigravityHandler(agClient, ms)
		mux.HandleFunc("GET /api/providers/antigravity/accounts", agH.ListAccounts)
		mux.HandleFunc("POST /api/providers/antigravity/accounts", agH.CreateAccount)
		mux.HandleFunc("PUT /api/providers/antigravity/accounts/{id}", agH.UpdateAccount)
		mux.HandleFunc("DELETE /api/providers/antigravity/accounts/{id}", agH.DeleteAccount)
		mux.HandleFunc("POST /api/providers/antigravity/accounts/{id}/refresh", agH.RefreshAccount)
		mux.HandleFunc("GET /api/providers/antigravity/oauth/auth-url", agH.GetOAuthURL)
		mux.HandleFunc("GET /api/providers/antigravity/models", agH.ListModels)
		mux.HandleFunc("PUT /api/providers/antigravity/models", agH.UpdateModels)
		mux.HandleFunc("POST /api/providers/antigravity/models/refresh", agH.RefreshModels)
		mux.HandleFunc("GET /api/providers/antigravity/accounts/{id}/quota", agH.GetAccountQuota)
		mux.HandleFunc("GET /api/providers/antigravity/quota", agH.GetAllQuotas)
	}

	if len(staticFS) > 0 && staticFS[0] != nil {
		mux.Handle("/", spaHandler(staticFS[0]))
	}

	return corsMiddleware(authMiddleware(authSvc, mux))
}

func authMiddleware(authSvc *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authSvc == nil || !authSvc.IsAuthEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path

		// Public endpoints that never require session authentication
		if strings.HasPrefix(path, "/api/auth/") ||
			path == "/api/health" ||
			path == "/callback" ||
			strings.HasPrefix(path, "/v1/") ||
			!strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// API key authenticated inference on /api/chat/completions
		if path == "/api/chat/completions" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
				if strings.HasPrefix(token, "lun-") {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		token := ExtractSessionToken(r)
		if token == "" || !authSvc.ValidateSession(token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Authentication required. Please log in.",
					"type":    "authentication_error",
				},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func spaHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServerFS(distFS)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1/") {
			http.NotFound(w, r)
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" {
			serveIndex(w, r, distFS)
			return
		}

		f, err := distFS.Open(cleanPath)
		if err != nil {
			serveIndex(w, r, distFS)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			serveIndex(w, r, distFS)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, distFS fs.FS) {
	indexFile, err := distFS.Open("index.html")
	if err != nil {
		http.Error(w, "Frontend not built. Please run pnpm build or pnpm compile.", http.StatusNotFound)
		return
	}
	defer indexFile.Close()

	stat, err := indexFile.Stat()
	if err != nil {
		http.Error(w, "Failed to stat index.html", http.StatusInternalServerError)
		return
	}

	if rs, ok := indexFile.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, indexFile)
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
