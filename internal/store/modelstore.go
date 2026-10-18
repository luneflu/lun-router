package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"lun-router/internal/model"
)

type CustomModel struct {
	ID                  string                   `json:"id"`
	Provider            string                   `json:"provider"`
	Name                string                   `json:"name,omitempty"`
	Description         string                   `json:"description,omitempty"`
	ContextLength       int                      `json:"context_length,omitempty"`
	MaxCompletionTokens int                      `json:"max_completion_tokens,omitempty"`
	Capabilities        *model.ModelCapabilities `json:"capabilities,omitempty"`
	ThinkingLevels      []string                 `json:"thinking_levels,omitempty"`
	CreatedAt           time.Time                `json:"created_at"`
}

type ModelStore struct {
	mu             sync.RWMutex
	filePath       string
	customFilePath string
	// map of provider -> map of modelID -> isDisabled
	disabled map[string]map[string]bool
	// map of provider -> map of cleanModelID -> CustomModel
	custom map[string]map[string]CustomModel
}

func NewModelStore(dataDir string) (*ModelStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	fp := filepath.Join(dataDir, "disabled_models.json")
	cfp := filepath.Join(dataDir, "custom_models.json")
	ms := &ModelStore{
		filePath:       fp,
		customFilePath: cfp,
		disabled:       make(map[string]map[string]bool),
		custom:         make(map[string]map[string]CustomModel),
	}

	if err := ms.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load disabled models: %w", err)
	}
	if err := ms.loadCustom(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load custom models: %w", err)
	}

	return ms, nil
}

func (ms *ModelStore) load() error {
	data, err := os.ReadFile(ms.filePath)
	if err != nil {
		return err
	}
	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	ms.disabled = make(map[string]map[string]bool)
	for prov, models := range raw {
		ms.disabled[prov] = make(map[string]bool)
		for _, m := range models {
			ms.disabled[prov][m] = true
		}
	}
	return nil
}

func (ms *ModelStore) saveLocked() error {
	raw := make(map[string][]string)
	for prov, models := range ms.disabled {
		for m, dis := range models {
			if dis {
				raw[prov] = append(raw[prov], m)
			}
		}
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := ms.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, ms.filePath)
}

func (ms *ModelStore) IsDisabled(provider, model string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if provModels, ok := ms.disabled[provider]; ok {
		return provModels[model]
	}
	return false
}

func (ms *ModelStore) SetDisabled(provider, model string, disabled bool) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, ok := ms.disabled[provider]; !ok {
		ms.disabled[provider] = make(map[string]bool)
	}
	if disabled {
		ms.disabled[provider][model] = true
	} else {
		delete(ms.disabled[provider], model)
	}
	return ms.saveLocked()
}

func (ms *ModelStore) SetBatchDisabled(provider string, models []string, disabled bool) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, ok := ms.disabled[provider]; !ok {
		ms.disabled[provider] = make(map[string]bool)
	}
	for _, m := range models {
		if disabled {
			ms.disabled[provider][m] = true
		} else {
			delete(ms.disabled[provider], m)
		}
	}
	return ms.saveLocked()
}

func (ms *ModelStore) loadCustom() error {
	data, err := os.ReadFile(ms.customFilePath)
	if err != nil {
		return err
	}
	var raw map[string][]CustomModel
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	ms.custom = make(map[string]map[string]CustomModel)
	for prov, models := range raw {
		ms.custom[prov] = make(map[string]CustomModel)
		for _, m := range models {
			clean := cleanProviderModelID(prov, m.ID)
			m.ID = clean
			m.Provider = prov
			ms.custom[prov][clean] = m
		}
	}
	return nil
}

func (ms *ModelStore) saveCustomLocked() error {
	raw := make(map[string][]CustomModel)
	for prov, modelsMap := range ms.custom {
		for _, m := range modelsMap {
			raw[prov] = append(raw[prov], m)
		}
		sort.Slice(raw[prov], func(i, j int) bool {
			return raw[prov][i].ID < raw[prov][j].ID
		})
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := ms.customFilePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, ms.customFilePath)
}

func cleanProviderModelID(provider, id string) string {
	clean := strings.TrimSpace(id)
	prefix := provider + "/"
	if strings.HasPrefix(clean, prefix) {
		clean = strings.TrimPrefix(clean, prefix)
	}
	return clean
}

func (ms *ModelStore) ListCustomModels(provider string) []CustomModel {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	provModels, ok := ms.custom[provider]
	if !ok || len(provModels) == 0 {
		return nil
	}

	res := make([]CustomModel, 0, len(provModels))
	for _, m := range provModels {
		res = append(res, m)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID < res[j].ID
	})
	return res
}

func (ms *ModelStore) HasCustomModel(provider, modelID string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	clean := cleanProviderModelID(provider, modelID)
	if provModels, ok := ms.custom[provider]; ok {
		_, exists := provModels[clean]
		return exists
	}
	return false
}

func (ms *ModelStore) GetCustomModel(provider, modelID string) (*CustomModel, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	clean := cleanProviderModelID(provider, modelID)
	if provModels, ok := ms.custom[provider]; ok {
		cm, exists := provModels[clean]
		if exists {
			copy := cm
			return &copy, true
		}
	}
	return nil, false
}

func (ms *ModelStore) AddCustomModel(provider string, cm CustomModel) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	clean := cleanProviderModelID(provider, cm.ID)
	if clean == "" {
		return fmt.Errorf("model id cannot be empty")
	}

	cm.ID = clean
	cm.Provider = provider
	if cm.CreatedAt.IsZero() {
		cm.CreatedAt = time.Now()
	}

	if _, ok := ms.custom[provider]; !ok {
		ms.custom[provider] = make(map[string]CustomModel)
	}
	ms.custom[provider][clean] = cm

	// Re-enable in disabled store if previously disabled
	if provDis, ok := ms.disabled[provider]; ok {
		delete(provDis, clean)
		_ = ms.saveLocked()
	}

	return ms.saveCustomLocked()
}

func (ms *ModelStore) DeleteCustomModel(provider, modelID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	clean := cleanProviderModelID(provider, modelID)
	if provModels, ok := ms.custom[provider]; ok {
		delete(provModels, clean)
	}
	return ms.saveCustomLocked()
}
