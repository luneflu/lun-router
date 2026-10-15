package handler

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"lun-router/internal/antigravity"
	"lun-router/internal/opencode"
	"lun-router/internal/store"
)

func NewRouter(ks *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, staticFS ...fs.FS) http.Handler {
	mux := http.NewServeMux()

	keyH := NewKeyHandler(ks)
	compH := NewCompletionHandler(ks, ms, client, agClient)

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
		mux.HandleFunc("GET /api/providers/antigravity/accounts/{id}/quota", agH.GetAccountQuota)
		mux.HandleFunc("GET /api/providers/antigravity/quota", agH.GetAllQuotas)
	}

	if len(staticFS) > 0 && staticFS[0] != nil {
		mux.Handle("/", spaHandler(staticFS[0]))
	}

	return corsMiddleware(mux)
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
