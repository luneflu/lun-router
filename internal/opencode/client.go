package opencode

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	BaseURL    = "https://opencode.ai"
	UA         = "opencode/1.18.35"
	ClientType = "cli"
	Project    = "global"
	Request    = "_msg"
)

var FallbackModels = []string{
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

type Client struct {
	baseURL        string
	httpClient     *http.Client
	sessionManager *SessionManager
	versionManager *VersionManager
}

func NewClient(baseURL string, vm *VersionManager) *Client {
	if baseURL == "" {
		baseURL = BaseURL
	}
	if vm == nil {
		vm = NewVersionManager("", 0)
	}
	return &Client{
		baseURL:        baseURL,
		httpClient:     http.DefaultClient,
		sessionManager: NewSessionManager(30 * time.Minute),
		versionManager: vm,
	}
}

func (c *Client) SessionManager() *SessionManager {
	return c.sessionManager
}

func (c *Client) VersionManager() *VersionManager {
	return c.versionManager
}

// FetchModels retrieves the catalog from OpenCode Free
func (c *Client) FetchModels(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/zen/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.versionManager.GetUserAgent())
	req.Header.Set("x-opencode-client", ClientType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream models status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// ExecuteChat sends chat request to OpenCode Free with Issue #4781 bypass headers.
// If customSession is provided, it uses it; otherwise uses managed session.
// If upstream returns HTTP 429 Too Many Requests (session quota limit),
// it automatically rotates the session for this identity and retries once.
func (c *Client) ExecuteChat(ctx context.Context, bodyMap map[string]any, identity string, customSession string) (*http.Response, error) {
	if identity == "" {
		identity = "default"
	}

	model, _ := bodyMap["model"].(string)
	if model == "" {
		model = "big-pickle"
		bodyMap["model"] = model
	}

	// Upstream always requires stream: true
	bodyMap["stream"] = true

	isResponses := strings.HasPrefix(model, "muse-spark")
	if isResponses {
		if maxTok, hasMax := bodyMap["max_tokens"]; hasMax {
			if _, hasOutput := bodyMap["max_output_tokens"]; !hasOutput {
				bodyMap["max_output_tokens"] = maxTok
			}
			delete(bodyMap, "max_tokens")
		}
		delete(bodyMap, "max_completion_tokens")

		var input []map[string]any

		switch msgs := bodyMap["messages"].(type) {
		case []any:
			for _, m := range msgs {
				if mMap, ok := m.(map[string]any); ok {
					role, _ := mMap["role"].(string)
					content, _ := mMap["content"].(string)
					if item, valid := toResponsesMessage(role, content); valid {
						input = append(input, item)
					}
				}
			}
		case []map[string]string:
			for _, mMap := range msgs {
				if item, valid := toResponsesMessage(mMap["role"], mMap["content"]); valid {
					input = append(input, item)
				}
			}
		case []map[string]any:
			for _, mMap := range msgs {
				role, _ := mMap["role"].(string)
				content, _ := mMap["content"].(string)
				if item, valid := toResponsesMessage(role, content); valid {
					input = append(input, item)
				}
			}
		}

		if len(input) == 0 {
			input = []map[string]any{
				{
					"type": "message",
					"role": "user",
					"content": []map[string]any{
						{"type": "input_text", "text": ""},
					},
				},
			}
		}

		bodyMap["input"] = input
		delete(bodyMap, "messages")
		bodyMap["store"] = false
	} else {
		delete(bodyMap, "max_output_tokens")
	}

	// Apply required fingerprint tools (read and shell for v2)
	ApplyFingerprint(bodyMap, isResponses)

	targetURL := c.baseURL + "/inference/v1/chat/completions"
	if isResponses {
		targetURL = c.baseURL + "/inference/v1/responses"
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	sessionID := customSession
	if sessionID == "" {
		sessionID = c.sessionManager.Get(identity)
	}

	resp, err := c.doRequest(ctx, targetURL, payloadBytes, sessionID)
	if err != nil {
		return nil, err
	}

	// If rate-limited (429): rotate session and immediately retry once with fresh session
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()

		newSessionID := c.sessionManager.Rotate(identity)
		return c.doRequest(ctx, targetURL, payloadBytes, newSessionID)
	}

	return resp, nil
}

func (c *Client) doRequest(ctx context.Context, targetURL string, payloadBytes []byte, sessionID string) (*http.Response, error) {
	upReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Aligned Custom Headers for OpenCode Free v2 (/inference/v1)
	upReq.Header.Set("User-Agent", c.versionManager.GetUserAgent())
	upReq.Header.Set("x-opencode-client", ClientType)
	upReq.Header.Set("x-opencode-project", Project)
	upReq.Header.Set("x-opencode-session", sessionID)
	upReq.Header.Set("x-opencode-session-id", sessionID)
	upReq.Header.Set("x-session-affinity", sessionID)
	upReq.Header.Set("X-Session-Id", sessionID)
	upReq.Header.Set("Authorization", "Bearer public")
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")

	return c.httpClient.Do(upReq)
}

// AggregateSSEStream parses incoming SSE stream into a standard OpenAI ChatCompletion response
func AggregateSSEStream(r io.Reader, fallbackModel string) (map[string]any, error) {
	var fullText strings.Builder
	var lastID string
	var lastModel = fallbackModel
	var lastCreated = time.Now().Unix()
	var finishReason = "stop"

	scanner := bufio.NewScanner(r)
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
			// Responses API fields
			Type     string `json:"type"`
			Delta    any    `json:"delta"`
			Response struct {
				ID string `json:"id"`
			} `json:"response"`
		}

		if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
			if chunk.ID != "" {
				lastID = chunk.ID
			}
			if chunk.Response.ID != "" {
				lastID = chunk.Response.ID
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
			// Parse Responses API stream deltas
			if chunk.Delta != nil {
				if dMap, ok := chunk.Delta.(map[string]any); ok {
					if t, ok := dMap["text"].(string); ok {
						fullText.WriteString(t)
					}
				} else if dStr, ok := chunk.Delta.(string); ok {
					fullText.WriteString(dStr)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read stream: %w", err)
	}

	if lastID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		lastID = "chatcmpl-" + hex.EncodeToString(b)
	}

	return map[string]any{
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
	}, nil
}

func toResponsesMessage(role, content string) (map[string]any, bool) {
	if role == "assistant" && strings.TrimSpace(content) == "" {
		return nil, false
	}
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}
	return map[string]any{
		"type": "message",
		"role": role,
		"content": []map[string]any{
			{"type": contentType, "text": content},
		},
	}, true
}
