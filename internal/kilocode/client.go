package kilocode

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
	BaseURL            = "https://api.kilo.ai"
	DeviceAuthURL      = "https://api.kilo.ai/api/device-auth/codes"
	ProfileURL         = "https://api.kilo.ai/api/profile"
	ChatCompletionsURL = "https://api.kilo.ai/api/openrouter/chat/completions"
	GatewayModelsURL   = "https://api.kilo.ai/api/gateway/models"
)

var FallbackModels = []string{
	"anthropic/claude-sonnet-4-20250514",
	"anthropic/claude-opus-4-20250514",
	"google/gemini-2.5-pro",
	"google/gemini-2.5-flash",
	"openai/gpt-4.1",
	"openai/o3",
	"deepseek/deepseek-chat",
	"deepseek/deepseek-reasoner",
}

type Client struct {
	store      *AccountStore
	httpClient *http.Client
	modelsFile string

	mu           sync.RWMutex
	models       []string
	freeModels   []GatewayModelItem
	modelMap     map[string]bool
	lastSyncedAt time.Time
}

func NewClient(store *AccountStore, dataDir string) *Client {
	c := &Client{
		store: store,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		modelsFile: filepath.Join(dataDir, "kilocode_models.json"),
		models:     append([]string{}, FallbackModels...),
		modelMap:   make(map[string]bool),
	}

	for _, m := range FallbackModels {
		c.modelMap[m] = true
	}

	c.loadCachedModels()
	return c
}

func (c *Client) Store() *AccountStore {
	return c.store
}

// RequestDeviceCode initiates device code flow on https://api.kilo.ai/api/device-auth/codes
func (c *Client) RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DeviceAuthURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute device code request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("Too many pending authorization requests. Please try again later.")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device auth initiation failed (status %d): %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Code            string `json:"code"`
		VerificationURL string `json:"verificationUrl"`
		ExpiresIn       int    `json:"expiresIn"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode device code response: %w", err)
	}

	expiresIn := raw.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 300
	}

	return &DeviceCodeResponse{
		DeviceCode:              raw.Code,
		UserCode:                raw.Code,
		VerificationURI:         raw.VerificationURL,
		VerificationURIComplete: raw.VerificationURL,
		ExpiresIn:               expiresIn,
		Interval:                3,
	}, nil
}

// PollToken checks the status of device authorization. If approved, it retrieves the user profile and creates an account.
func (c *Client) PollToken(ctx context.Context, deviceCode string) (*Account, string, error) {
	pollURL := fmt.Sprintf("%s/%s", DeviceAuthURL, deviceCode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return nil, "poll_failed", fmt.Errorf("create poll request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "poll_failed", fmt.Errorf("execute poll request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted { // 202
		return nil, "pending", nil
	}
	if resp.StatusCode == http.StatusForbidden { // 403
		return nil, "access_denied", errors.New("authorization denied by user")
	}
	if resp.StatusCode == http.StatusGone { // 410
		return nil, "expired_token", errors.New("authorization code expired")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "poll_failed", fmt.Errorf("poll failed: %d", resp.StatusCode)
	}

	var raw struct {
		Status    string `json:"status"`
		Token     string `json:"token"`
		UserEmail string `json:"userEmail"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, "poll_failed", fmt.Errorf("decode poll response: %w", err)
	}

	if raw.Status != "approved" || raw.Token == "" {
		return nil, "pending", nil
	}

	// Fetch profile to get orgId for X-Kilocode-OrganizationID header and fallback email
	profile, err := c.VerifyToken(ctx, raw.Token)
	var orgID string
	userEmail := raw.UserEmail
	if err == nil && profile != nil {
		if len(profile.Organizations) > 0 {
			orgID = profile.Organizations[0].ID
		}
		if userEmail == "" && profile.Email != "" {
			userEmail = profile.Email
		}
	}
	if userEmail == "" {
		userEmail = "kilocode-user"
	}

	acc := &Account{
		Email:       userEmail,
		OrgID:       orgID,
		AccessToken: raw.Token,
		IsActive:    true,
		CreatedAt:   time.Now().UTC(),
	}

	if err := c.store.Save(acc); err != nil {
		return nil, "save_failed", fmt.Errorf("save account: %w", err)
	}

	return acc, "approved", nil
}

// VerifyToken calls https://api.kilo.ai/api/profile with the given token
func (c *Client) VerifyToken(ctx context.Context, token string) (*ProfileResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ProfileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute profile request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("profile request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var profile ProfileResponse
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode profile response: %w", err)
	}

	return &profile, nil
}

// AddManualAccount saves a manually provided API token
func (c *Client) AddManualAccount(ctx context.Context, token, email, orgID string) (*Account, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("token cannot be empty")
	}

	profile, err := c.VerifyToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if email == "" && profile.Email != "" {
		email = profile.Email
	}
	if email == "" {
		email = "manual@kilocode"
	}

	if orgID == "" && len(profile.Organizations) > 0 {
		orgID = profile.Organizations[0].ID
	}

	acc := &Account{
		Email:       email,
		OrgID:       orgID,
		AccessToken: token,
		IsActive:    true,
		CreatedAt:   time.Now().UTC(),
	}

	if err := c.store.Save(acc); err != nil {
		return nil, fmt.Errorf("save account: %w", err)
	}

	return acc, nil
}

// TestAccount tests the validity of a saved account by calling the profile API
func (c *Client) TestAccount(ctx context.Context, id string) (int64, error) {
	acc, ok := c.store.Get(id)
	if !ok {
		return 0, fmt.Errorf("account not found: %s", id)
	}

	start := time.Now()
	_, err := c.VerifyToken(ctx, acc.AccessToken)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return latency, err
	}
	return latency, nil
}

// FetchModels queries https://api.kilo.ai/api/gateway/models and updates catalog
func (c *Client) FetchModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GatewayModelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute models request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models request failed (status %d)", resp.StatusCode)
	}

	var res GatewayModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	modelSet := make(map[string]bool)
	var modelList []string
	var freeList []GatewayModelItem

	// Add fallbacks first
	for _, m := range FallbackModels {
		if !modelSet[m] {
			modelSet[m] = true
			modelList = append(modelList, m)
		}
	}

	// Add gateway models
	for _, item := range res.Data {
		if item.ID != "" && !modelSet[item.ID] {
			modelSet[item.ID] = true
			modelList = append(modelList, item.ID)
		}
		if item.IsFree {
			freeList = append(freeList, item)
		}
	}

	c.mu.Lock()
	c.models = modelList
	c.modelMap = modelSet
	c.freeModels = freeList
	c.lastSyncedAt = time.Now().UTC()
	c.mu.Unlock()

	c.saveCachedModels(modelList, freeList)
	return modelList, nil
}

func (c *Client) loadCachedModels() {
	data, err := os.ReadFile(c.modelsFile)
	if err != nil {
		return
	}
	var cached struct {
		Models     []string           `json:"models"`
		FreeModels []GatewayModelItem `json:"freeModels"`
	}
	if err := json.Unmarshal(data, &cached); err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(cached.Models) > 0 {
		c.models = cached.Models
		c.modelMap = make(map[string]bool)
		for _, m := range cached.Models {
			c.modelMap[m] = true
		}
	}
	if len(cached.FreeModels) > 0 {
		c.freeModels = cached.FreeModels
	}
}

func (c *Client) saveCachedModels(models []string, freeModels []GatewayModelItem) {
	cached := struct {
		Models     []string           `json:"models"`
		FreeModels []GatewayModelItem `json:"freeModels"`
		UpdatedAt  time.Time          `json:"updatedAt"`
	}{
		Models:     models,
		FreeModels: freeModels,
		UpdatedAt:  time.Now().UTC(),
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(c.modelsFile, data, 0644)
}

// GetCatalogModels returns the list of all available Kilo Code models
func (c *Client) GetCatalogModels() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string{}, c.models...)
}

// GetFreeModels returns free models matching 9router /api/providers/kilo/free-models route
func (c *Client) GetFreeModels() []GatewayModelItem {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]GatewayModelItem{}, c.freeModels...)
}

// IsCatalogModel checks if a model ID is recognized
func (c *Client) IsCatalogModel(model string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	clean := CleanModelID(model)
	return c.modelMap[clean]
}

// StartBackgroundSync syncs the models catalog periodically
func (c *Client) StartBackgroundSync(ctx context.Context) {
	go func() {
		// Initial sync
		if _, err := c.FetchModels(ctx); err != nil {
			log.Printf("[KiloCode] Initial models sync: %v (using fallbacks)", err)
		} else {
			log.Printf("[KiloCode] Models catalog synced: %d models available", len(c.GetCatalogModels()))
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := c.FetchModels(ctx); err != nil {
					log.Printf("[KiloCode] Background models sync error: %v", err)
				}
			}
		}
	}()
}

// HasKilocodePrefix checks if a model name has a kilo/kilocode/kc prefix
func HasKilocodePrefix(model string) bool {
	return strings.HasPrefix(model, "kilocode/") ||
		strings.HasPrefix(model, "kilo/") ||
		strings.HasPrefix(model, "kc/")
}

// CleanModelID strips any provider prefix from a model name
func CleanModelID(model string) string {
	if strings.HasPrefix(model, "kilocode/") {
		return strings.TrimPrefix(model, "kilocode/")
	}
	if strings.HasPrefix(model, "kilo/") {
		return strings.TrimPrefix(model, "kilo/")
	}
	if strings.HasPrefix(model, "kc/") {
		return strings.TrimPrefix(model, "kc/")
	}
	return model
}

// ExecuteChat sends chat completion request to Kilo Code OpenRouter gateway
func (c *Client) ExecuteChat(ctx context.Context, body map[string]any, stream bool) (*http.Response, *Account, error) {
	acc, ok := c.store.GetActive()
	if !ok {
		return nil, nil, errors.New("no active Kilo Code account found; please add or activate an account in Providers > Kilo Code")
	}

	modelStr, _ := body["model"].(string)
	cleanModel := CleanModelID(modelStr)

	// Clone body map and ensure clean model
	bodyCopy := make(map[string]any, len(body))
	for k, v := range body {
		bodyCopy[k] = v
	}
	bodyCopy["model"] = cleanModel
	bodyCopy["stream"] = stream

	reqBodyBytes, err := json.Marshal(bodyCopy)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ChatCompletionsURL, bytes.NewReader(reqBodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("create chat completions request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
	if acc.OrgID != "" {
		req.Header.Set("X-Kilocode-OrganizationID", acc.OrgID)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("execute chat completions request: %w", err)
	}

	return resp, acc, nil
}
