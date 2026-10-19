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
func (c *Client) FetchModels(ctx context.Context, p Provider) ([]string, error) {
	baseURL := strings.TrimRight(p.BaseURL, "/")
	modelsURL := baseURL + "/models"

	models, err := c.tryFetchModels(ctx, modelsURL, p.APIKey)
	if err == nil {
		return models, nil
	}

	// Fallback: If 404 and baseURL doesn't have /v1, try baseURL + "/v1/models"
	if !strings.Contains(baseURL, "/v1") {
		fallbackURL := baseURL + "/v1/models"
		if fbModels, fbErr := c.tryFetchModels(ctx, fallbackURL, p.APIKey); fbErr == nil {
			return fbModels, nil
		}
	}

	return nil, err
}

func (c *Client) tryFetchModels(ctx context.Context, url string, apiKey string) ([]string, error) {
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

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	var results []string
	seen := make(map[string]bool)

	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			results = append(results, id)
		}
	}

	for _, item := range parsed.Models {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = strings.TrimSpace(item.Name)
		}
		if id != "" && !seen[id] {
			seen[id] = true
			results = append(results, id)
		}
	}

	sort.Strings(results)
	return results, nil
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
