package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"lun-router/internal/antigravity"
	"lun-router/internal/auth"
	"lun-router/internal/caveman"
	"lun-router/internal/chatgpt"
	"lun-router/internal/compactor"
	"lun-router/internal/kilocode"
	"lun-router/internal/opencode"
	"lun-router/internal/ponytail"
	"lun-router/internal/store"
	"lun-router/internal/usage"
	"lun-router/internal/zai"
)

func NewRouter(ks *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, compactorSvc *compactor.Service, cavemanSvc *caveman.Service, ponytailSvc *ponytail.Service, authSvc *auth.Service, usageSvc *usage.Service, staticFS ...fs.FS) http.Handler {
	return NewRouterWithProviders(ks, ms, client, agClient, nil, nil, compactorSvc, cavemanSvc, ponytailSvc, authSvc, usageSvc, staticFS...)
}

// NewRouterWithChatGPT keeps the original two-provider signature working.
func NewRouterWithChatGPT(ks *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, cgClient *chatgpt.Client, compactorSvc *compactor.Service, cavemanSvc *caveman.Service, ponytailSvc *ponytail.Service, authSvc *auth.Service, usageSvc *usage.Service, staticFS ...fs.FS) http.Handler {
	return NewRouterWithProviders(ks, ms, client, agClient, cgClient, nil, compactorSvc, cavemanSvc, ponytailSvc, authSvc, usageSvc, staticFS...)
}

// NewRouterWithProviders wires every provider, including z.ai.
func NewRouterWithProviders(ks *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, cgClient *chatgpt.Client, zaiClient *zai.Client, compactorSvc *compactor.Service, cavemanSvc *caveman.Service, ponytailSvc *ponytail.Service, authSvc *auth.Service, usageSvc *usage.Service, staticFS ...fs.FS) http.Handler {
	return NewRouterWithAllProviders(ks, ms, client, agClient, cgClient, zaiClient, nil, compactorSvc, cavemanSvc, ponytailSvc, authSvc, usageSvc, staticFS...)
}

// NewRouterWithAllProviders wires all providers including Kilo Code.
func NewRouterWithAllProviders(ks *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, cgClient *chatgpt.Client, zaiClient *zai.Client, kcClient *kilocode.Client, compactorSvc *compactor.Service, cavemanSvc *caveman.Service, ponytailSvc *ponytail.Service, authSvc *auth.Service, usageSvc *usage.Service, staticFS ...fs.FS) http.Handler {

	mux := http.NewServeMux()

	keyH := NewKeyHandler(ks)
	compH := NewCompletionHandler(ks, ms, client, agClient, compactorSvc, cavemanSvc, ponytailSvc)
	if cgClient != nil {
		compH.SetChatGPT(cgClient)
	}
	if zaiClient != nil {
		compH.SetZai(zaiClient)
	}
	if kcClient != nil {
		compH.SetKiloCode(kcClient)
	}
	compactH := NewCompactorHandler(compactorSvc)
	usageH := NewUsageHandler(usageSvc)
	compH.SetUsage(usageSvc)
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

	// Usage Routes (request/token monitoring)
	mux.HandleFunc("GET /api/usage", usageH.Summary)

	// Compactor Routes (tool output compression)
	mux.HandleFunc("GET /api/compactor", compactH.GetStatus)
	mux.HandleFunc("PUT /api/compactor", compactH.Update)
	mux.HandleFunc("POST /api/compactor/test", compactH.TestCompress)

	// Caveman Routes (Prompt & Voice Optimization)
	mux.HandleFunc("GET /api/caveman", caveH.GetStatus)
	mux.HandleFunc("PUT /api/caveman", caveH.UpdateConfig)
	mux.HandleFunc("POST /api/caveman/test", caveH.TestPrompt)

	// Ponytail Routes (Lazy Senior Developer & Anti-Bloat)
	mux.HandleFunc("GET /api/ponytail", ponyH.GetStatus)
	mux.HandleFunc("PUT /api/ponytail", ponyH.UpdateConfig)
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
	mux.HandleFunc("POST /api/providers/opencode/models/custom", compH.AddCustomModel)
	mux.HandleFunc("DELETE /api/providers/opencode/models/custom/{id...}", compH.DeleteCustomModel)
	mux.HandleFunc("DELETE /api/providers/opencode/models/custom", compH.DeleteCustomModel)
	mux.HandleFunc("POST /api/providers/opencode/version/refresh", compH.RefreshVersion)
	mux.HandleFunc("GET /api/providers/opencode/proxy", compH.OpenCodeProxyStatus)
	mux.HandleFunc("PUT /api/providers/opencode/proxy", compH.UpdateOpenCodeProxy)
	mux.HandleFunc("POST /api/providers/opencode/proxy/refresh", compH.RefreshOpenCodeProxy)
	mux.HandleFunc("GET /api/providers/chatgpt/models", compH.ChatGPTProviderModels)
	mux.HandleFunc("PUT /api/providers/chatgpt/models", compH.UpdateChatGPTProviderModels)
	mux.HandleFunc("GET /api/providers/chatgpt/status", compH.ChatGPTStatus)
	mux.HandleFunc("PUT /api/providers/chatgpt/status", compH.UpdateChatGPTStatus)
	mux.HandleFunc("POST /api/providers/chatgpt/install", compH.InstallChatGPTBrowser)
	mux.HandleFunc("POST /api/providers/chatgpt/cookies", compH.UploadChatGPTCookies)
	mux.HandleFunc("DELETE /api/providers/chatgpt/cookies", compH.DeleteChatGPTCookies)
	mux.HandleFunc("PUT /api/providers/chatgpt/tools", compH.UpdateChatGPTTools)
	mux.HandleFunc("GET /api/providers/zai/models", compH.ZaiProviderModels)
	mux.HandleFunc("PUT /api/providers/zai/models", compH.UpdateZaiProviderModels)
	mux.HandleFunc("GET /api/providers/zai/status", compH.ZaiStatus)
	mux.HandleFunc("PUT /api/providers/zai/status", compH.UpdateZaiStatus)
	mux.HandleFunc("POST /api/providers/zai/install", compH.InstallZaiBrowser)
	mux.HandleFunc("PUT /api/providers/zai/tools", compH.UpdateZaiTools)
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
		mux.HandleFunc("POST /api/providers/antigravity/models/custom", agH.AddCustomModel)
		mux.HandleFunc("DELETE /api/providers/antigravity/models/custom/{id...}", agH.DeleteCustomModel)
		mux.HandleFunc("DELETE /api/providers/antigravity/models/custom", agH.DeleteCustomModel)
		mux.HandleFunc("GET /api/providers/antigravity/accounts/{id}/quota", agH.GetAccountQuota)
		mux.HandleFunc("GET /api/providers/antigravity/quota", agH.GetAllQuotas)
	}

	// Kilo Code Provider Routes (9router parity)
	if kcClient != nil {
		kcH := NewKiloCodeHandler(kcClient, ms)
		mux.HandleFunc("GET /api/providers/kilocode/accounts", kcH.ListAccounts)
		mux.HandleFunc("POST /api/providers/kilocode/accounts", kcH.CreateAccount)
		mux.HandleFunc("PUT /api/providers/kilocode/accounts/{id}", kcH.UpdateAccount)
		mux.HandleFunc("DELETE /api/providers/kilocode/accounts/{id}", kcH.DeleteAccount)
		mux.HandleFunc("POST /api/providers/kilocode/accounts/{id}/test", kcH.TestAccount)
		mux.HandleFunc("POST /api/providers/kilocode/oauth/device-code", kcH.RequestDeviceCode)
		mux.HandleFunc("GET /api/providers/kilocode/oauth/device-code", kcH.RequestDeviceCode)
		mux.HandleFunc("GET /api/oauth/kilocode/device-code", kcH.RequestDeviceCode)
		mux.HandleFunc("POST /api/providers/kilocode/oauth/poll", kcH.Poll)
		mux.HandleFunc("GET /api/providers/kilocode/oauth/poll", kcH.Poll)
		mux.HandleFunc("POST /api/oauth/kilocode/poll", kcH.Poll)
		mux.HandleFunc("GET /api/providers/kilocode/models", kcH.ListModels)
		mux.HandleFunc("PUT /api/providers/kilocode/models", kcH.UpdateModels)
		mux.HandleFunc("POST /api/providers/kilocode/models/refresh", kcH.RefreshModels)
		mux.HandleFunc("POST /api/providers/kilocode/models/custom", kcH.AddCustomModel)
		mux.HandleFunc("DELETE /api/providers/kilocode/models/custom/{id...}", kcH.DeleteCustomModel)
		mux.HandleFunc("DELETE /api/providers/kilocode/models/custom", kcH.DeleteCustomModel)
		mux.HandleFunc("GET /api/providers/kilo/free-models", kcH.GetFreeModels)
	}

	// Unknown API paths must not fall through to the SPA handler, which answers
	// 200 text/html and makes clients fail with an opaque parse error.
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": fmt.Sprintf("Unknown endpoint %s %s on this router. Supported: POST /v1/chat/completions, GET /v1/models.", r.Method, r.URL.Path),
				"type":    "invalid_request_error",
			},
		})
	})

	if len(staticFS) > 0 && staticFS[0] != nil {
		mux.Handle("/", spaHandler(staticFS[0]))
	}

	return corsMiddleware(apiRequestLogger(authMiddleware(authSvc, mux)))
}

// apiRequestLogger records inbound API traffic when LUN_DEBUG_REQUESTS=1.
// It exists to answer "is the client even reaching this router, and on which
// endpoint?" when an agent UI appears to reply with nothing.
func apiRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("LUN_DEBUG_REQUESTS") != "1" || !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		var bodyPreview string
		if r.Body != nil {
			raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			r.Body = io.NopCloser(bytes.NewReader(raw))
			preview := raw
			if len(preview) > 400 {
				preview = preview[:400]
			}
			bodyPreview = string(preview)
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		fmt.Fprintf(os.Stderr, "[req] %s %s ua=%q auth=%v body=%s\n",
			r.Method, r.URL.Path, r.Header.Get("User-Agent"), r.Header.Get("Authorization") != "", bodyPreview)
		next.ServeHTTP(rec, r)
		fmt.Fprintf(os.Stderr, "[req] %s %s -> %d\n", r.Method, r.URL.Path, rec.status)
	})
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
			strings.HasPrefix(path, "/api/oauth/") ||
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
