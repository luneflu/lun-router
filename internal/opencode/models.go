package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ModelCache struct {
	Models    []string  `json:"models"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ModelManager struct {
	mu            sync.RWMutex
	models        []string
	rawJSON       []byte
	cacheFilePath string
	checkInterval time.Duration
	client        *Client
}

func NewModelManager(dataDir string, client *Client, interval time.Duration) *ModelManager {
	if interval <= 0 {
		interval = 1 * time.Hour
	}

	var cachePath string
	if dataDir != "" {
		cachePath = filepath.Join(dataDir, "opencode_models.json")
	}

	mm := &ModelManager{
		models:        append([]string{}, FallbackModels...),
		cacheFilePath: cachePath,
		checkInterval: interval,
		client:        client,
	}

	mm.loadCache()
	return mm
}

func (mm *ModelManager) loadCache() {
	if mm.cacheFilePath == "" {
		return
	}
	data, err := os.ReadFile(mm.cacheFilePath)
	if err != nil {
		return
	}
	var cached ModelCache
	if err := json.Unmarshal(data, &cached); err == nil && len(cached.Models) > 0 {
		mm.mu.Lock()
		mm.models = cached.Models
		mm.mu.Unlock()
	}
}

func (mm *ModelManager) saveCache(models []string) {
	if mm.cacheFilePath == "" {
		return
	}
	cached := ModelCache{
		Models:    models,
		UpdatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(mm.cacheFilePath, data, 0644)
}

func (mm *ModelManager) GetModels() []string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	res := make([]string, len(mm.models))
	copy(res, mm.models)
	return res
}

func (mm *ModelManager) GetRawJSON() []byte {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	if len(mm.rawJSON) > 0 {
		res := make([]byte, len(mm.rawJSON))
		copy(res, mm.rawJSON)
		return res
	}
	return nil
}

func (mm *ModelManager) CheckAndUpdate(ctx context.Context) ([]string, bool, error) {
	if mm.client == nil {
		return mm.GetModels(), false, fmt.Errorf("client not initialized")
	}

	rawBytes, err := mm.client.FetchModelsDirect(ctx)
	if err != nil {
		return mm.GetModels(), false, err
	}

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBytes, &resp); err != nil {
		return mm.GetModels(), false, fmt.Errorf("parse upstream models: %w", err)
	}

	if len(resp.Data) == 0 {
		return mm.GetModels(), false, fmt.Errorf("empty model list from upstream")
	}

	var newModels []string
	seen := make(map[string]bool)
	for _, m := range resp.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			newModels = append(newModels, id)
		}
	}

	// Always ensure fallback models are included
	for _, fb := range FallbackModels {
		if !seen[fb] {
			seen[fb] = true
			newModels = append(newModels, fb)
		}
	}

	mm.mu.Lock()
	updated := len(newModels) != len(mm.models)
	mm.models = newModels
	mm.rawJSON = rawBytes
	mm.mu.Unlock()

	if updated {
		mm.saveCache(newModels)
	}

	return newModels, updated, nil
}

func (mm *ModelManager) StartBackgroundSync(ctx context.Context) {
	go func() {
		// Initial check after 3 seconds
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
			if models, updated, err := mm.CheckAndUpdate(ctx); err == nil && updated {
				log.Printf("[OpenCode] Models synced from upstream: %d models available", len(models))
			}
		}

		ticker := time.NewTicker(mm.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if models, updated, err := mm.CheckAndUpdate(ctx); err == nil && updated {
					log.Printf("[OpenCode] Models refreshed from upstream: %d models available", len(models))
				}
			}
		}
	}()
}
