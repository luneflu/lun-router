package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lun-router/internal/opencode"
	"lun-router/internal/store"
)

type CompletionHandler struct {
	store      *store.KeyStore
	modelStore *store.ModelStore
	client     *opencode.Client
}

func NewCompletionHandler(s *store.KeyStore, ms *store.ModelStore, client *opencode.Client) *CompletionHandler {
	if client == nil {
		client = opencode.NewClient("", nil)
	}
	return &CompletionHandler{
		store:      s,
		modelStore: ms,
		client:     client,
	}
}

type ModelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
	Enabled bool   `json:"enabled"`
}

// GET /v1/models (OpenAI-compatible catalog: returns only enabled models)
func (h *CompletionHandler) Models(w http.ResponseWriter, r *http.Request) {
	rawModels := h.fetchAllModels(r)

	var filtered []ModelItem
	for _, m := range rawModels {
		if h.modelStore == nil || !h.modelStore.IsDisabled("opencode", m.ID) {
			filtered = append(filtered, m)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   filtered,
	})
}

// GET /api/providers/opencode/models (Provider view: returns all models with enabled status)
func (h *CompletionHandler) ProviderModels(w http.ResponseWriter, r *http.Request) {
	rawModels := h.fetchAllModels(r)

	for i := range rawModels {
		rawModels[i].Enabled = h.modelStore == nil || !h.modelStore.IsDisabled("opencode", rawModels[i].ID)
	}

	version := opencode.DefaultVersion
	if h.client != nil && h.client.VersionManager() != nil {
		version = h.client.VersionManager().GetVersion()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "opencode",
		"version":  version,
		"models":   rawModels,
	})
}

// POST /api/providers/opencode/version/refresh (Check upstream GitHub for version updates)
func (h *CompletionHandler) RefreshVersion(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.VersionManager() == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Version manager not initialized"})
		return
	}

	version, updated, err := h.client.VersionManager().CheckAndUpdate(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"version": version,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"version": version,
		"updated": updated,
	})
}

// PUT /api/providers/opencode/models (Toggle single or batch model enabled/disabled)
func (h *CompletionHandler) UpdateProviderModels(w http.ResponseWriter, r *http.Request) {
	if h.modelStore == nil {
		http.Error(w, `{"error":"Model store not configured"}`, http.StatusInternalServerError)
		return
	}

	var payload struct {
		Model   string   `json:"model"`
		Models  []string `json:"models"`
		Enabled bool     `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	var err error
	if len(payload.Models) > 0 {
		err = h.modelStore.SetBatchDisabled("opencode", payload.Models, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled("opencode", payload.Model, !payload.Enabled)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Either model or models required"})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"enabled": payload.Enabled,
	})
}

// POST /api/models/test (Ping model with minimal probe like 9router to check if it works)
func (h *CompletionHandler) TestModel(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Model == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Model identifier required"})
		return
	}

	model := payload.Model
	if h.modelStore != nil && h.modelStore.IsDisabled("opencode", model) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         false,
			"model":      model,
			"error":      fmt.Sprintf("Model '%s' is disabled", model),
			"latency_ms": 0,
		})
		return
	}

	prompt := payload.Prompt
	if prompt == "" {
		prompt = "ping"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	testBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream":            true,
		"max_tokens":        16,
		"max_output_tokens": 16,
	}

	customSession := r.Header.Get("x-opencode-session")
	if customSession == "" {
		customSession = r.Header.Get("x-opencode-session-id")
	}

	upResp, err := h.client.ExecuteChat(ctx, testBody, "model-test", customSession)
	if err != nil {
		latencyMs := time.Since(start).Milliseconds()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         false,
			"model":      model,
			"error":      err.Error(),
			"latency_ms": latencyMs,
		})
		return
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusOK {
		latencyMs := time.Since(start).Milliseconds()
		bodyBytes, _ := io.ReadAll(upResp.Body)
		var errObj struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		errMsg := string(bodyBytes)
		if json.Unmarshal(bodyBytes, &errObj) == nil && errObj.Error.Message != "" {
			errMsg = errObj.Error.Message
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":         false,
			"model":      model,
			"error":      errMsg,
			"latency_ms": latencyMs,
			"status":     upResp.StatusCode,
		})
		return
	}

	// Read initial stream byte to confirm TTFT
	reader := bufio.NewReader(upResp.Body)
	_, _ = reader.ReadBytes('\n')
	latencyMs := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"model":      model,
		"latency_ms": latencyMs,
	})
}

func (h *CompletionHandler) fetchAllModels(r *http.Request) []ModelItem {
	data, err := h.client.FetchModels(r.Context())
	if err == nil {
		var resp struct {
			Data []ModelItem `json:"data"`
		}
		if jsonErr := json.Unmarshal(data, &resp); jsonErr == nil && len(resp.Data) > 0 {
			return resp.Data
		}
	}

	// Fallback models if upstream is unreachable
	var list []ModelItem
	for _, m := range opencode.FallbackModels {
		list = append(list, ModelItem{
			ID:      m,
			Object:  "model",
			OwnedBy: "opencode-free",
		})
	}
	return list
}

func (h *CompletionHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	// 1. Authenticate with router API key
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader {
		token = r.URL.Query().Get("key")
	}

	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Missing API key. Provide Authorization: Bearer <key>",
				"type":    "authentication_error",
			},
		})
		return
	}

	key, ok := h.store.FindByKey(token)
	if !ok || !key.IsActive {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Invalid or deactivated API key",
				"type":    "authentication_error",
			},
		})
		return
	}

	// 2. Decode incoming request body
	var bodyMap map[string]any
	if err := json.NewDecoder(r.Body).Decode(&bodyMap); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Invalid JSON request body",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	model, _ := bodyMap["model"].(string)
	if model == "" {
		model = "big-pickle"
		bodyMap["model"] = model
	}

	// Check if requested model is disabled
	if h.modelStore != nil && h.modelStore.IsDisabled("opencode", model) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": fmt.Sprintf("Model '%s' is disabled for provider opencode", model),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	clientWantsStream, _ := bodyMap["stream"].(bool)

	// 3. Execute with OpenCode client
	customSession := r.Header.Get("x-opencode-session")
	if customSession == "" {
		customSession = r.Header.Get("x-opencode-session-id")
	}

	upResp, err := h.client.ExecuteChat(r.Context(), bodyMap, token, customSession)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": fmt.Sprintf("Upstream OpenCode error: %v", err),
				"type":    "api_error",
			},
		})
		return
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upResp.StatusCode)
		scanner := bufio.NewScanner(upResp.Body)
		for scanner.Scan() {
			_, _ = w.Write(scanner.Bytes())
		}
		return
	}

	// 4. Stream response to client if requested
	if clientWantsStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, canFlush := w.(http.Flusher)
		reader := bufio.NewReader(upResp.Body)

		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				_, _ = w.Write(line)
				if canFlush {
					flusher.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		return
	}

	// 5. Aggregate streaming chunks into full JSON response
	fullResp, err := opencode.AggregateSSEStream(upResp.Body, model)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": fmt.Sprintf("Failed to aggregate stream: %v", err),
				"type":    "api_error",
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fullResp)
}
