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
	UA         = "opencode/1.18.31"
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
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = BaseURL
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
	}
}

// FetchModels retrieves the catalog from OpenCode Free
func (c *Client) FetchModels(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/zen/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UA)
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

// ExecuteChat sends chat request to OpenCode Free with Issue #4781 bypass headers
func (c *Client) ExecuteChat(ctx context.Context, bodyMap map[string]any) (*http.Response, error) {
	model, _ := bodyMap["model"].(string)
	if model == "" {
		model = "big-pickle"
		bodyMap["model"] = model
	}

	// Upstream always requires stream: true
	bodyMap["stream"] = true

	// Apply required fingerprint tools
	ApplyFingerprint(bodyMap)

	targetURL := c.baseURL + "/zen/v1/chat/completions"
	if strings.HasPrefix(model, "muse-spark") {
		targetURL = c.baseURL + "/zen/v1/responses"
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	upReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// 5 Custom Headers for OpenCode Free bypass (GitHub Issue #4781)
	upReq.Header.Set("User-Agent", UA)
	upReq.Header.Set("x-opencode-client", ClientType)
	upReq.Header.Set("x-opencode-project", Project)
	upReq.Header.Set("x-opencode-request", Request)
	upReq.Header.Set("x-opencode-session", GenerateSessionID())
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
