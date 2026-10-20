package commandcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	BaseURL     = "https://api.commandcode.ai"
	GenerateURL = BaseURL + "/alpha/generate"
	WhoamiURL   = BaseURL + "/alpha/whoami"
	ModelsURL   = BaseURL + "/provider/v1/models?view=client"

	// CLIVersion is the command-code release whose /alpha/generate contract this
	// client mirrors. Bump it when the npm package changes its wire format.
	CLIVersion = "1.84.0"
)

// FallbackModels are advertised when the catalog has never been fetched.
var FallbackModels = []string{
	"deepseek/deepseek-v4-pro",
	"deepseek/deepseek-v4-flash",
	"moonshotai/Kimi-K2.7-Code",
	"moonshotai/Kimi-K2.6",
	"zai-org/GLM-5.2",
	"MiniMaxAI/MiniMax-M3",
	"xiaomi/mimo-v2.5-pro",
	"Qwen/Qwen3.7-Max",
	"claude-sonnet-4-6",
	"gpt-5.4-mini",
}

type Client struct {
	store      *AccountStore
	httpClient *http.Client
	modelsFile string
	workingDir string

	mu       sync.RWMutex
	models   []CatalogModel
	lastSync time.Time
}

func NewClient(store *AccountStore, dataDir string) *Client {
	c := &Client{
		store: store,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
		modelsFile: filepath.Join(dataDir, "commandcode_models.json"),
	}
	if wd, err := os.Getwd(); err == nil {
		c.workingDir = wd
	}
	c.loadCachedModels()
	return c
}

func (c *Client) Store() *AccountStore { return c.store }

// headers mirrors buildCommandAuthHeaders from command-code@1.84.0.
func (c *Client) headers(apiKey, sessionID string) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "cli")
	h.Set("x-command-code-version", CLIVersion)
	h.Set("x-cli-environment", "production")
	h.Set("x-project-slug", projectSlug(c.workingDir))
	h.Set("x-taste-learning", "false")
	h.Set("x-session-id", sessionID)
	if apiKey != "" {
		h.Set("Authorization", "Bearer "+apiKey)
	}
	return h
}

func projectSlug(workingDir string) string {
	if workingDir == "" {
		return "lun-router"
	}
	return filepath.Base(workingDir)
}

// VerifyKey checks a CLI API key against /alpha/whoami and returns the account name.
func (c *Client) VerifyKey(ctx context.Context, apiKey string) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", errors.New("API key is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, WhoamiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create whoami request: %w", err)
	}
	for k, v := range c.headers(apiKey, newUUID()) {
		req.Header[k] = v
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute whoami request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("invalid Command Code API key (status %d): %s", resp.StatusCode, ErrorMessage(body))
	}

	var who struct {
		User struct {
			UserName string `json:"userName"`
			Email    string `json:"email"`
		} `json:"user"`
	}
	_ = json.Unmarshal(body, &who)
	if who.User.UserName != "" {
		return who.User.UserName, nil
	}
	return who.User.Email, nil
}

// AddManualAccount verifies and stores a CLI API key.
func (c *Client) AddManualAccount(ctx context.Context, apiKey, name string) (*Account, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("API key is required")
	}

	whoName, err := c.VerifyKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = whoName
	}
	if name == "" {
		name = "Command Code"
	}

	acc := &Account{Name: name, APIKey: apiKey, IsActive: true, CreatedAt: time.Now().UTC()}
	if err := c.store.Save(acc); err != nil {
		return nil, fmt.Errorf("save account: %w", err)
	}
	return acc, nil
}

// TestAccount measures whoami latency for a stored key.
func (c *Client) TestAccount(ctx context.Context, id string) (int64, error) {
	acc, ok := c.store.Get(id)
	if !ok {
		return 0, fmt.Errorf("account not found: %s", id)
	}

	start := time.Now()
	_, err := c.VerifyKey(ctx, acc.APIKey)
	return time.Since(start).Milliseconds(), err
}

// ExecuteChat posts an OpenAI body to /alpha/generate. Upstream is always streamed;
// the caller relays it with Relay.
func (c *Client) ExecuteChat(ctx context.Context, body map[string]any, stream bool) (*http.Response, *Account, error) {
	acc, ok := c.store.GetActive()
	if !ok {
		return nil, nil, errors.New("no active Command Code account found; add an API key in Providers > Command Code")
	}

	wire, err := BuildBody(body, c.workingDir)
	if err != nil {
		return nil, nil, err
	}
	if stream {
		if params, ok := wire["params"].(map[string]any); ok {
			params["stream"] = true
		}
	}

	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request body: %w", err)
	}

	if os.Getenv("COMMANDCODE_DEBUG_BODY") == "1" {
		log.Printf("[CommandCode] /alpha/generate body: %s", payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GenerateURL, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("create generate request: %w", err)
	}
	for k, v := range c.headers(acc.APIKey, newUUID()) {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("execute generate request: %w", err)
	}
	return resp, acc, nil
}

// FetchModels refreshes the model catalog from /provider/v1/models?view=client.
func (c *Client) FetchModels(ctx context.Context) ([]CatalogModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ModelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}
	apiKey := ""
	if acc, ok := c.store.GetActive(); ok {
		apiKey = acc.APIKey
	}
	for k, v := range c.headers(apiKey, newUUID()) {
		req.Header[k] = v
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute models request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("models request failed (status %d): %s", resp.StatusCode, ErrorMessage(body))
	}

	var catalog struct {
		Data []CatalogModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	models := make([]CatalogModel, 0, len(catalog.Data))
	for _, m := range catalog.Data {
		if m.ID == "" || m.Hidden {
			continue
		}
		models = append(models, m)
	}

	c.mu.Lock()
	c.models = models
	c.lastSync = time.Now().UTC()
	c.mu.Unlock()

	c.saveCachedModels(models)
	return models, nil
}

// GetCatalogModels returns the cached catalog, falling back to FallbackModels.
func (c *Client) GetCatalogModels() []CatalogModel {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.models) > 0 {
		return append([]CatalogModel{}, c.models...)
	}
	fallback := make([]CatalogModel, 0, len(FallbackModels))
	for _, id := range FallbackModels {
		fallback = append(fallback, CatalogModel{ID: id, Name: id})
	}
	return fallback
}

func (c *Client) IsCatalogModel(model string) bool {
	clean := CleanModelID(model)
	for _, m := range c.GetCatalogModels() {
		if m.ID == clean {
			return true
		}
	}
	return false
}

func (c *Client) loadCachedModels() {
	data, err := os.ReadFile(c.modelsFile)
	if err != nil {
		return
	}
	var cached struct {
		Models []CatalogModel `json:"models"`
	}
	if err := json.Unmarshal(data, &cached); err != nil || len(cached.Models) == 0 {
		return
	}

	c.mu.Lock()
	c.models = cached.Models
	c.mu.Unlock()
}

func (c *Client) saveCachedModels(models []CatalogModel) {
	data, err := json.MarshalIndent(struct {
		Models    []CatalogModel `json:"models"`
		UpdatedAt time.Time      `json:"updatedAt"`
	}{Models: models, UpdatedAt: time.Now().UTC()}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(c.modelsFile, data, 0644)
}

// StartBackgroundSync keeps the catalog fresh; it is public so a cold start works
// without an API key.
func (c *Client) StartBackgroundSync(ctx context.Context) {
	go func() {
		if _, err := c.FetchModels(ctx); err != nil {
			log.Printf("[CommandCode] Initial models sync: %v (using fallbacks)", err)
		} else {
			log.Printf("[CommandCode] Models catalog synced: %d models available", len(c.GetCatalogModels()))
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := c.FetchModels(ctx); err != nil {
					log.Printf("[CommandCode] Background models sync error: %v", err)
				}
			}
		}
	}()
}

// HasCommandCodePrefix reports model IDs namespaced to this provider.
func HasCommandCodePrefix(model string) bool {
	return strings.HasPrefix(model, "commandcode/") || strings.HasPrefix(model, "cmc/")
}

// CleanModelID strips the provider namespace.
func CleanModelID(model string) string {
	if strings.HasPrefix(model, "commandcode/") {
		return strings.TrimPrefix(model, "commandcode/")
	}
	if strings.HasPrefix(model, "cmc/") {
		return strings.TrimPrefix(model, "cmc/")
	}
	return model
}

// ErrorMessage extracts the server's error message from
// {"success":false,"error":{"message":...}} or {"error":{"message":...}}.
func ErrorMessage(body []byte) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error.Message != "" {
		if payload.Error.Code != "" {
			return payload.Error.Code + ": " + payload.Error.Message
		}
		return payload.Error.Message
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500]
	}
	if text == "" {
		text = "empty response body"
	}
	return text
}
