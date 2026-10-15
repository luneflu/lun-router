package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"lun-router/internal/antigravity"
	"lun-router/internal/caveman"
	"lun-router/internal/model"
	"lun-router/internal/opencode"
	"lun-router/internal/ponytail"
	"lun-router/internal/rtk"
	"lun-router/internal/store"
)

type CompletionHandler struct {
	store           *store.KeyStore
	modelStore      *store.ModelStore
	client          *opencode.Client
	agClient        *antigravity.Client
	rtkService      *rtk.Service
	cavemanService  *caveman.Service
	ponytailService *ponytail.Service
}

func NewCompletionHandler(s *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, rtkSvc *rtk.Service, cavemanSvc *caveman.Service, ponytailSvc *ponytail.Service) *CompletionHandler {
	if client == nil {
		client = opencode.NewClient("", nil)
	}
	return &CompletionHandler{
		store:           s,
		modelStore:      ms,
		client:          client,
		agClient:        agClient,
		rtkService:      rtkSvc,
		cavemanService:  cavemanSvc,
		ponytailService: ponytailSvc,
	}
}

type ModelItem = model.ModelItem
type ModelCapabilities = model.ModelCapabilities

// GET /v1/models (OpenAI-compatible catalog: returns only enabled models)
func (h *CompletionHandler) Models(w http.ResponseWriter, r *http.Request) {
	rawModels := h.fetchAllModels(r)

	var filtered []ModelItem
	for _, m := range rawModels {
		if h.modelStore == nil || !h.modelStore.IsDisabled("opencode", m.ID) {
			filtered = append(filtered, m)
		}
	}

	// Antigravity models
	if h.agClient != nil {
		for _, m := range h.agClient.GetCatalogModels() {
			if h.modelStore == nil || !h.modelStore.IsDisabled("antigravity", m) {
				item := ModelItem{
					ID:      m,
					Object:  "model",
					OwnedBy: "antigravity",
					Enabled: true,
				}
				model.EnrichModelItem(&item, "antigravity")
				filtered = append(filtered, item)
			}
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

// POST /api/providers/opencode/models/refresh (Sync latest models from OpenCode Free upstream)
func (h *CompletionHandler) RefreshOpenCodeModels(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.ModelManager() == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Model manager not initialized"})
		return
	}

	models, updated, err := h.client.ModelManager().CheckAndUpdate(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"count":   len(models),
			"updated": false,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"count":   len(models),
		"updated": updated,
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

type ModelTestResult struct {
	OK        bool   `json:"ok"`
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	Status    int    `json:"status,omitempty"`
}

type ModelTestPayload struct {
	Model       string   `json:"model,omitempty"`
	Models      []string `json:"models,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Concurrency int      `json:"concurrency,omitempty"`
}

func (h *CompletionHandler) pingSingleModel(ctx context.Context, model string, prompt string, customSession string) ModelTestResult {
	if prompt == "" {
		prompt = "ping"
	}

	// Antigravity Model Test
	if antigravity.IsAntigravityModel(model) {
		cleanModel := antigravity.CleanModelID(model)
		if h.modelStore != nil && h.modelStore.IsDisabled("antigravity", cleanModel) {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     fmt.Sprintf("Model '%s' is disabled for provider antigravity", model),
				LatencyMs: 0,
			}
		}

		if h.agClient == nil {
			return ModelTestResult{
				OK:    false,
				Model: model,
				Error: "Antigravity provider not initialized",
			}
		}

		mCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		start := time.Now()
		testBody := map[string]any{
			"model": cleanModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		}

		upResp, err := h.agClient.ExecuteChat(mCtx, testBody, false)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     err.Error(),
				LatencyMs: latencyMs,
			}
		}
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(upResp.Body)
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     string(bodyBytes),
				LatencyMs: latencyMs,
				Status:    upResp.StatusCode,
			}
		}

		return ModelTestResult{
			OK:        true,
			Model:     model,
			LatencyMs: latencyMs,
		}
	}

	// OpenCode Model Test
	if h.modelStore != nil && h.modelStore.IsDisabled("opencode", model) {
		return ModelTestResult{
			OK:        false,
			Model:     model,
			Error:     fmt.Sprintf("Model '%s' is disabled", model),
			LatencyMs: 0,
		}
	}

	mCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
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

	upResp, err := h.client.ExecuteChat(mCtx, testBody, "model-test", customSession)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return ModelTestResult{
			OK:        false,
			Model:     model,
			Error:     err.Error(),
			LatencyMs: latencyMs,
		}
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusOK {
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
		return ModelTestResult{
			OK:        false,
			Model:     model,
			Error:     errMsg,
			LatencyMs: latencyMs,
			Status:    upResp.StatusCode,
		}
	}

	// Read initial stream byte to confirm TTFT
	reader := bufio.NewReader(upResp.Body)
	_, _ = reader.ReadBytes('\n')
	latencyMs = time.Since(start).Milliseconds()

	return ModelTestResult{
		OK:        true,
		Model:     model,
		LatencyMs: latencyMs,
	}
}

// POST /api/models/test (Ping model or batch of models)
func (h *CompletionHandler) TestModel(w http.ResponseWriter, r *http.Request) {
	var payload ModelTestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Invalid request body"})
		return
	}

	customSession := r.Header.Get("x-opencode-session")
	if customSession == "" {
		customSession = r.Header.Get("x-opencode-session-id")
	}

	// Batch mode
	if len(payload.Models) > 0 {
		concurrency := payload.Concurrency
		if concurrency <= 0 {
			concurrency = 4
		} else if concurrency > 10 {
			concurrency = 10
		}

		results := make([]ModelTestResult, len(payload.Models))
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for i, m := range payload.Models {
			wg.Add(1)
			go func(idx int, modelID string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				results[idx] = h.pingSingleModel(r.Context(), modelID, payload.Prompt, customSession)
			}(i, m)
		}

		wg.Wait()

		passed := 0
		failed := 0
		for _, res := range results {
			if res.OK {
				passed++
			} else {
				failed++
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      failed == 0,
			"results": results,
			"total":   len(results),
			"passed":  passed,
			"failed":  failed,
		})
		return
	}

	// Single model mode
	if payload.Model == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Model or models identifier required"})
		return
	}

	res := h.pingSingleModel(r.Context(), payload.Model, payload.Prompt, customSession)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *CompletionHandler) fetchAllModels(r *http.Request) []ModelItem {
	data, err := h.client.FetchModels(r.Context())
	if err == nil {
		var resp struct {
			Data []ModelItem `json:"data"`
		}
		if jsonErr := json.Unmarshal(data, &resp); jsonErr == nil && len(resp.Data) > 0 {
			for i := range resp.Data {
				resp.Data[i].OwnedBy = "opencode"
				model.EnrichModelItem(&resp.Data[i], "opencode")
			}
			return resp.Data
		}
	}

	// Fallback models from ModelManager or FallbackModels
	var fallbackIDs []string
	if h.client != nil && h.client.ModelManager() != nil {
		fallbackIDs = h.client.ModelManager().GetModels()
	} else {
		fallbackIDs = opencode.FallbackModels
	}

	var list []ModelItem
	for _, m := range fallbackIDs {
		item := ModelItem{
			ID:      m,
			Object:  "model",
			OwnedBy: "opencode",
		}
		model.EnrichModelItem(&item, "opencode")
		list = append(list, item)
	}
	return list
}

func (h *CompletionHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	// 1. Authenticate with router API key
	isInternalAPI := strings.HasPrefix(r.URL.Path, "/api/chat/completions")

	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader {
		token = r.URL.Query().Get("key")
	}

	if token == "" {
		if !isInternalAPI {
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
		token = "web-ui-test"
	} else {
		key, ok := h.store.FindByKey(token)
		if (!ok || !key.IsActive) && !isInternalAPI {
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

	clientWantsStream, _ := bodyMap["stream"].(bool)

	// RTK: auto-compress tool output before dispatching to upstream LLMs
	// Client opt-out via X-RTK: off, X-9Router-Token-Saver: off, or X-Token-Saver: off
	clientOptOut := strings.EqualFold(r.Header.Get("X-RTK"), "off") ||
		strings.EqualFold(r.Header.Get("X-9Router-Token-Saver"), "off") ||
		strings.EqualFold(r.Header.Get("X-Token-Saver"), "off") ||
		strings.EqualFold(r.URL.Query().Get("rtk"), "off") ||
		strings.EqualFold(r.URL.Query().Get("token_saver"), "off")

	if h.rtkService != nil {
		h.rtkService.ProcessBody(bodyMap, clientOptOut)
	}

	// Caveman voice & prompt optimization (say less, cut output tokens)
	headerCaveman := r.Header.Get("X-Caveman")
	headerMode := r.Header.Get("X-Caveman-Mode")
	if clientOptOut {
		headerCaveman = "off"
	}
	if h.cavemanService != nil {
		h.cavemanService.Apply(bodyMap, headerCaveman, headerMode)
	}

	// Ponytail lazy senior developer mode (minimal code, anti-bloat)
	headerPonytail := r.Header.Get("X-Ponytail")
	headerPonytailMode := r.Header.Get("X-Ponytail-Mode")
	if clientOptOut {
		headerPonytail = "off"
	}
	if h.ponytailService != nil {
		h.ponytailService.Apply(bodyMap, headerPonytail, headerPonytailMode)
	}

	// Check if this is an Antigravity model
	if antigravity.IsAntigravityModel(model) {
		cleanModel := antigravity.CleanModelID(model)
		if h.modelStore != nil && h.modelStore.IsDisabled("antigravity", cleanModel) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Model '%s' is disabled for provider antigravity", model),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if h.agClient == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Antigravity provider not initialized",
					"type":    "api_error",
				},
			})
			return
		}

		upResp, err := h.agClient.ExecuteChat(r.Context(), bodyMap, clientWantsStream)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Upstream Antigravity error: %v", err),
					"type":    "api_error",
				},
			})
			return
		}
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(upResp.StatusCode)
			bodyBytes, _ := io.ReadAll(upResp.Body)
			_, _ = w.Write(bodyBytes)
			return
		}

		if clientWantsStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, canFlush := w.(http.Flusher)
			streamAntigravityAsOpenAI(w, flusher, canFlush, upResp.Body, model)
			return
		}

		handleAntigravityNonStreamResponse(w, upResp.Body, model)
		return
	}

	// Check if requested model is disabled for OpenCode
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

		isResponses := strings.HasPrefix(model, "muse-spark")
		if isResponses {
			streamResponsesAsOpenAI(w, flusher, canFlush, upResp.Body, model)
			return
		}

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

func streamResponsesAsOpenAI(w io.Writer, flusher http.Flusher, canFlush bool, body io.Reader, model string) {
	reader := bufio.NewReader(body)
	streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	sentRole := false

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "data: ") {
				payload := strings.TrimPrefix(trimmed, "data: ")
				if payload == "[DONE]" {
					_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
					if canFlush {
						flusher.Flush()
					}
					break
				}

				var raw map[string]any
				if json.Unmarshal([]byte(payload), &raw) == nil {
					if _, hasChoices := raw["choices"]; hasChoices {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
						if canFlush {
							flusher.Flush()
						}
					} else {
						msgType, _ := raw["type"].(string)

						// 1. First assistant delta: send role chunk if needed
						if !sentRole && (msgType == "response.output_text.delta" || msgType == "response.output_item.added") {
							roleChunk, _ := json.Marshal(map[string]any{
								"id":      streamID,
								"object":  "chat.completion.chunk",
								"created": created,
								"model":   model,
								"choices": []map[string]any{
									{
										"index": 0,
										"delta": map[string]string{
											"role": "assistant",
										},
									},
								},
							})
							_, _ = fmt.Fprintf(w, "data: %s\n\n", roleChunk)
							if canFlush {
								flusher.Flush()
							}
							sentRole = true
						}

						// 2. Text delta
						if msgType == "response.output_text.delta" {
							var textDelta string
							if dStr, ok := raw["delta"].(string); ok {
								textDelta = dStr
							} else if dMap, ok := raw["delta"].(map[string]any); ok {
								if t, ok := dMap["text"].(string); ok {
									textDelta = t
								}
							}

							if textDelta != "" {
								chunk, _ := json.Marshal(map[string]any{
									"id":      streamID,
									"object":  "chat.completion.chunk",
									"created": created,
									"model":   model,
									"choices": []map[string]any{
										{
											"index": 0,
											"delta": map[string]string{
												"content": textDelta,
											},
										},
									},
								})
								_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
								if canFlush {
									flusher.Flush()
								}
							}
						} else if msgType == "response.completed" {
							// 3. Final chunk with finish_reason: "stop"
							finishChunk, _ := json.Marshal(map[string]any{
								"id":      streamID,
								"object":  "chat.completion.chunk",
								"created": created,
								"model":   model,
								"choices": []map[string]any{
									{
										"index":         0,
										"delta":         map[string]string{},
										"finish_reason": "stop",
									},
								},
							})
							_, _ = fmt.Fprintf(w, "data: %s\n\n", finishChunk)
							_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
							if canFlush {
								flusher.Flush()
							}
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
}

func streamAntigravityAsOpenAI(w io.Writer, flusher http.Flusher, canFlush bool, body io.Reader, model string) {
	reader := bufio.NewReader(body)
	streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	sentRole := false

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "data: ") {
				payload := strings.TrimPrefix(trimmed, "data: ")
				if payload == "[DONE]" {
					_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
					if canFlush {
						flusher.Flush()
					}
					break
				}

				var data map[string]any
				if jsonErr := json.Unmarshal([]byte(payload), &data); jsonErr == nil {
					// Handle Antigravity envelope
					respMap, ok := data["response"].(map[string]any)
					if !ok {
						respMap = data
					}

					candidates, _ := respMap["candidates"].([]any)
					if len(candidates) > 0 {
						if cand, ok := candidates[0].(map[string]any); ok {
							// 1. Initial role chunk
							if !sentRole {
								sentRole = true
								roleChunk, _ := json.Marshal(map[string]any{
									"id":      streamID,
									"object":  "chat.completion.chunk",
									"created": created,
									"model":   model,
									"choices": []map[string]any{
										{
											"index": 0,
											"delta": map[string]string{
												"role": "assistant",
											},
										},
									},
								})
								_, _ = fmt.Fprintf(w, "data: %s\n\n", roleChunk)
								if canFlush {
									flusher.Flush()
								}
							}

							// 2. Text parts
							if content, ok := cand["content"].(map[string]any); ok {
								if parts, ok := content["parts"].([]any); ok {
									for _, pItem := range parts {
										if pMap, ok := pItem.(map[string]any); ok {
											if text, ok := pMap["text"].(string); ok && text != "" {
												chunk, _ := json.Marshal(map[string]any{
													"id":      streamID,
													"object":  "chat.completion.chunk",
													"created": created,
													"model":   model,
													"choices": []map[string]any{
														{
															"index": 0,
															"delta": map[string]string{
																"content": text,
															},
														},
													},
												})
												_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
												if canFlush {
													flusher.Flush()
												}
											}
										}
									}
								}
							}

							// 3. Finish reason
							finishReason, _ := cand["finishReason"].(string)
							if finishReason != "" {
								finishChunk, _ := json.Marshal(map[string]any{
									"id":      streamID,
									"object":  "chat.completion.chunk",
									"created": created,
									"model":   model,
									"choices": []map[string]any{
										{
											"index":         0,
											"delta":         map[string]string{},
											"finish_reason": "stop",
										},
									},
								})
								_, _ = fmt.Fprintf(w, "data: %s\n\n", finishChunk)
								_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
								if canFlush {
									flusher.Flush()
								}
								break
							}
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
}

func handleAntigravityNonStreamResponse(w http.ResponseWriter, body io.Reader, model string) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read response"})
		return
	}

	var data map[string]any
	if jsonErr := json.Unmarshal(bodyBytes, &data); jsonErr != nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bodyBytes)
		return
	}

	respMap, ok := data["response"].(map[string]any)
	if !ok {
		respMap = data
	}

	var fullText string
	if candidates, ok := respMap["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]any); ok {
					for _, pItem := range parts {
						if pMap, ok := pItem.(map[string]any); ok {
							if text, ok := pMap["text"].(string); ok {
								fullText += text
							}
						}
					}
				}
			}
		}
	}

	openAIResp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": fullText,
				},
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openAIResp)
}

