package customprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// CleanModelID removes the provider prefix from model name if present.
func CleanModelID(prefix, modelID string) string {
	clean := strings.TrimSpace(modelID)
	prefixSlash := strings.ToLower(prefix) + "/"
	if strings.HasPrefix(strings.ToLower(clean), prefixSlash) {
		return clean[len(prefixSlash):]
	}
	return clean
}

// HasPrefix checks if a model name starts with the provider prefix.
func HasPrefix(prefix, modelID string) bool {
	prefixSlash := strings.ToLower(prefix) + "/"
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), prefixSlash)
}

// FetchModels queries the provider's /models endpoint to retrieve available models.
func (c *Client) FetchModels(ctx context.Context, p Provider) ([]Model, error) {
	baseURL := strings.TrimRight(p.BaseURL, "/")
	modelsURL := baseURL + "/models"

	models, err := c.tryFetchModels(ctx, modelsURL, p.APIKey, p.Compat)
	if err == nil {
		return models, nil
	}

	// Fallback: If 404 and baseURL doesn't have /v1, try baseURL + "/v1/models"
	if !strings.Contains(baseURL, "/v1") {
		fallbackURL := baseURL + "/v1/models"
		if fbModels, fbErr := c.tryFetchModels(ctx, fallbackURL, p.APIKey, p.Compat); fbErr == nil {
			return fbModels, nil
		}
	}

	return nil, err
}

func (c *Client) tryFetchModels(ctx context.Context, url string, apiKey string, compat *ProviderCompat) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}

	var parsed struct {
		Data   []map[string]any `json:"data"`
		Models []map[string]any `json:"models"`
	}

	var items []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err == nil && (len(parsed.Data) > 0 || len(parsed.Models) > 0) {
		items = append(items, parsed.Data...)
		items = append(items, parsed.Models...)
	} else {
		var rawList []map[string]any
		decoder2 := json.NewDecoder(bytes.NewReader(bodyBytes))
		decoder2.UseNumber()
		if err2 := decoder2.Decode(&rawList); err2 == nil {
			items = rawList
		} else {
			return nil, fmt.Errorf("decode models response: %w", err)
		}
	}

	var results []Model
	seen := make(map[string]bool)

	for _, item := range items {
		m, ok := ParseModelFromMap(item, compat)
		if !ok {
			continue
		}
		if !seen[m.ID] {
			seen[m.ID] = true
			results = append(results, m)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

func capabilitySupported(val any) *bool {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case bool:
		return &v
	case map[string]any:
		if sup, ok := v["supported"]; ok {
			return capabilitySupported(sup)
		}
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		if s == "true" {
			b := true
			return &b
		}
		if s == "false" {
			b := false
			return &b
		}
	}
	return nil
}

func extractString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		val, ok := m[k]
		if !ok || val == nil {
			continue
		}
		if s, ok := val.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func extractPositiveInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		val, ok := m[k]
		if !ok || val == nil {
			continue
		}
		switch v := val.(type) {
		case float64:
			if v > 0 && v == float64(int(v)) {
				return int(v)
			}
		case int:
			if v > 0 {
				return v
			}
		case int64:
			if v > 0 {
				return int(v)
			}
		case json.Number:
			if n, err := v.Int64(); err == nil && n > 0 {
				return int(n)
			}
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// ParseModelFromMap extracts Pi model fields from upstream model item:
// - name: name, display_name, then the model ID
// - contextWindow: context_window, contextWindow, max_model_len (vLLM), context_length, max_context_length, max_input_tokens (Anthropic)
// - maxTokens: max_tokens, maxTokens, max_output_tokens, max_completion_tokens
// - input: Always includes text. image is added when an explicit input array containing text and/or image lists it; otherwise when capabilities.vision or capabilities.image_input is supported
// - reasoning: Provider compat.reasoning, model reasoning, capabilities.reasoning, capabilities.thinking
func ParseModelFromMap(raw map[string]any, compat *ProviderCompat) (Model, bool) {
	id := extractString(raw, "id")
	if id == "" {
		id = extractString(raw, "name")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Model{}, false
	}

	name := extractString(raw, "name", "display_name")
	if name == "" {
		name = id
	}

	cw := extractPositiveInt(raw, "context_window", "contextWindow", "max_model_len", "context_length", "max_context_length", "max_input_tokens")
	mt := extractPositiveInt(raw, "max_tokens", "maxTokens", "max_output_tokens", "max_completion_tokens")

	var caps map[string]any
	if c, ok := raw["capabilities"].(map[string]any); ok {
		caps = c
	}

	var imageSupported *bool
	if rawInput, ok := raw["input"]; ok {
		switch slice := rawInput.(type) {
		case []any:
			hasTextOrImage := false
			hasImage := false
			for _, item := range slice {
				if s, ok := item.(string); ok {
					s = strings.ToLower(strings.TrimSpace(s))
					if s == "text" {
						hasTextOrImage = true
					} else if s == "image" {
						hasTextOrImage = true
						hasImage = true
					}
				}
			}
			if hasTextOrImage {
				imageSupported = &hasImage
			}
		case []string:
			hasTextOrImage := false
			hasImage := false
			for _, s := range slice {
				s = strings.ToLower(strings.TrimSpace(s))
				if s == "text" {
					hasTextOrImage = true
				} else if s == "image" {
					hasTextOrImage = true
					hasImage = true
				}
			}
			if hasTextOrImage {
				imageSupported = &hasImage
			}
		}
	}

	if imageSupported == nil && caps != nil {
		if v := capabilitySupported(caps["vision"]); v != nil {
			imageSupported = v
		} else if v := capabilitySupported(caps["image_input"]); v != nil {
			imageSupported = v
		}
	}

	input := []string{"text"}
	if imageSupported != nil && *imageSupported {
		input = append(input, "image")
	}

	var reasoning *bool
	if compat != nil && compat.Reasoning != nil {
		reasoning = compat.Reasoning
	} else if v := capabilitySupported(raw["reasoning"]); v != nil {
		reasoning = v
	} else if caps != nil {
		if v := capabilitySupported(caps["reasoning"]); v != nil {
			reasoning = v
		} else if v := capabilitySupported(caps["thinking"]); v != nil {
			reasoning = v
		}
	}

	return Model{
		ID:            id,
		Name:          name,
		ContextWindow: cw,
		MaxTokens:     mt,
		Input:         input,
		Reasoning:     reasoning,
	}, true
}

// ExecuteChat sends chat completion request to the custom provider's /chat/completions endpoint.
func (c *Client) ExecuteChat(ctx context.Context, p Provider, body map[string]any, stream bool) (*http.Response, error) {
	if !p.IsEnabled {
		return nil, fmt.Errorf("provider %q is disabled", p.Name)
	}

	baseURL := strings.TrimRight(p.BaseURL, "/")
	chatURL := baseURL + "/chat/completions"

	modelStr, _ := body["model"].(string)
	cleanModel := CleanModelID(p.Prefix, modelStr)
	if cleanModel == "" {
		return nil, errors.New("model parameter is required")
	}

	bodyCopy := make(map[string]any, len(body))
	for k, v := range body {
		bodyCopy[k] = v
	}
	bodyCopy["model"] = cleanModel
	bodyCopy["stream"] = stream

	reqBodyBytes, err := json.Marshal(bodyCopy)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL, bytes.NewReader(reqBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create chat completions request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute chat completions: %w", err)
	}

	return resp, nil
}
