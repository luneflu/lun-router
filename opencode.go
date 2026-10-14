package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	openCodeBaseURL = "https://opencode.ai"
	openCodeUA      = "opencode/1.18.31"
	openCodeClient  = "cli"
	openCodeProject = "global"
	openCodeRequest = "_msg"
	base62Chars     = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// Fallback models when offline or upstream is slow
var fallbackOpenCodeModels = []string{
	"big-pickle",
	"ling-3.1-flash-free",
	"cat-jam-free",
	"mimo-v2.6-flash-free",
	"nemotron-3.5-lightning-free",
	"longcat-2.5-preview-free",
	"space-bunny-free",
	"muse-spark-1.2-contributor-free",
	"muse-spark-1.3-contributor-free",
}

// Canonical OpenCode session generator: ses_<12 hex><14 base62>
// Fix for GitHub Issue #4781 (Bypass HTTP 403 FreeTierError)
func generateOpenCodeSessionID() string {
	nowMs := time.Now().UnixMilli()
	val := (^uint64(nowMs*4096 + 1)) & 0xFFFFFFFFFFFF
	timePart := fmt.Sprintf("%012x", val)

	randomBytes := make([]byte, 14)
	_, _ = rand.Read(randomBytes)
	var randomPart strings.Builder
	for _, b := range randomBytes {
		randomPart.WriteByte(base62Chars[int(b)%62])
	}

	return fmt.Sprintf("ses_%s%s", timePart, randomPart.String())
}

// Fingerprint tools required by upstream free tier gate
var openCodeFingerprintTools = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "bash",
			"description": "Execute a bash command in the terminal.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "glob",
			"description": "Find files matching pattern.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "grep",
			"description": "Search file contents.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "read",
			"description": "Read file contents.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	},
}

func applyOpenCodeFingerprint(bodyMap map[string]any) {
	existingTools, _ := bodyMap["tools"].([]any)
	hasTool := make(map[string]bool)

	for _, t := range existingTools {
		if tMap, ok := t.(map[string]any); ok {
			if fn, ok := tMap["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					hasTool[strings.ToLower(name)] = true
				}
			} else if name, ok := tMap["name"].(string); ok {
				hasTool[strings.ToLower(name)] = true
			}
		}
	}

	for _, ft := range openCodeFingerprintTools {
		fn := ft["function"].(map[string]any)
		name := fn["name"].(string)
		if !hasTool[name] {
			existingTools = append(existingTools, ft)
		}
	}

	bodyMap["tools"] = existingTools
}

// Handle /v1/models & /api/opencode/models
func handleOpenCodeModels(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequestWithContext(r.Context(), "GET", openCodeBaseURL+"/zen/v1/models", nil)
	if err == nil {
		req.Header.Set("User-Agent", openCodeUA)
		req.Header.Set("x-opencode-client", openCodeClient)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.Copy(w, resp.Body)
			return
		}
	}

	// Fallback response
	type ModelItem struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	var list []ModelItem
	for _, m := range fallbackOpenCodeModels {
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

// Handle /v1/chat/completions with OpenCode Free upstream proxy and issue #4781 bypass
func handleOpenCodeChatCompletions(ks *KeyStore, upstreamBase string) http.HandlerFunc {
	if upstreamBase == "" {
		upstreamBase = openCodeBaseURL
	}

	return func(w http.ResponseWriter, r *http.Request) {
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

		key, ok := ks.FindByKey(token)
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

		// OpenCode free upstream ALWAYS requires stream: true
		bodyMap["stream"] = true

		// Apply required fingerprint tools
		applyOpenCodeFingerprint(bodyMap)

		// Route endpoint
		targetURL := upstreamBase + "/zen/v1/chat/completions"
		if strings.HasPrefix(model, "muse-spark") {
			targetURL = upstreamBase + "/zen/v1/responses"
		}

		payloadBytes, err := json.Marshal(bodyMap)
		if err != nil {
			http.Error(w, `{"error":{"message":"Failed to serialize request"}}`, http.StatusInternalServerError)
			return
		}

		upReq, err := http.NewRequestWithContext(r.Context(), "POST", targetURL, bytes.NewReader(payloadBytes))
		if err != nil {
			http.Error(w, `{"error":{"message":"Failed to create upstream request"}}`, http.StatusInternalServerError)
			return
		}

		// 5 Custom Headers for OpenCode Free bypass (GitHub Issue #4781)
		sessionID := generateOpenCodeSessionID()
		upReq.Header.Set("User-Agent", openCodeUA)
		upReq.Header.Set("x-opencode-client", openCodeClient)
		upReq.Header.Set("x-opencode-project", openCodeProject)
		upReq.Header.Set("x-opencode-request", openCodeRequest)
		upReq.Header.Set("x-opencode-session", sessionID)
		upReq.Header.Set("Authorization", "Bearer public")
		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("Accept", "text/event-stream")

		upResp, err := http.DefaultClient.Do(upReq)
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
			_, _ = io.Copy(w, upResp.Body)
			return
		}

		// 3. Client wants streaming: stream SSE events directly
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

		// 4. Client wants non-streaming: aggregate streaming chunks into complete OpenAI ChatCompletion response
		var fullText strings.Builder
		var lastID string
		var lastModel = model
		var lastCreated = time.Now().Unix()
		var finishReason = "stop"

		scanner := bufio.NewScanner(upResp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				break
			}

			var chunk struct {
				ID      string `json:"id"`
				Model   string `json:"model"`
				Created int64  `json:"created"`
				Choices []struct {
					Delta struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
				if chunk.ID != "" {
					lastID = chunk.ID
				}
				if chunk.Model != "" {
					lastModel = chunk.Model
				}
				if chunk.Created > 0 {
					lastCreated = chunk.Created
				}
				if len(chunk.Choices) > 0 {
					fullText.WriteString(chunk.Choices[0].Delta.Content)
					if chunk.Choices[0].FinishReason != nil {
						finishReason = *chunk.Choices[0].FinishReason
					}
				}
			}
		}

		if lastID == "" {
			lastID = "chatcmpl-" + generateRandomHex(12)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      lastID,
			"object":  "chat.completion",
			"created": lastCreated,
			"model":   lastModel,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": fullText.String(),
					},
					"finish_reason": finishReason,
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     0,
				"completion_tokens": len(fullText.String()) / 4,
				"total_tokens":      len(fullText.String()) / 4,
			},
		})
	}
}
