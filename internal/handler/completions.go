package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"lun-router/internal/opencode"
	"lun-router/internal/store"
)

type CompletionHandler struct {
	store  *store.KeyStore
	client *opencode.Client
}

func NewCompletionHandler(s *store.KeyStore, client *opencode.Client) *CompletionHandler {
	if client == nil {
		client = opencode.NewClient("")
	}
	return &CompletionHandler{
		store:  s,
		client: client,
	}
}

func (h *CompletionHandler) Models(w http.ResponseWriter, r *http.Request) {
	data, err := h.client.FetchModels(r.Context())
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
		return
	}

	// Fallback response when offline
	type ModelItem struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	var list []ModelItem
	for _, m := range opencode.FallbackModels {
		list = append(list, ModelItem{
			ID:      m,
			Object:  "model",
			OwnedBy: "opencode-free",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   list,
	})
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

	clientWantsStream, _ := bodyMap["stream"].(bool)

	// 3. Execute with OpenCode client
	upResp, err := h.client.ExecuteChat(r.Context(), bodyMap)
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
