package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type ModelStore struct {
	mu       sync.RWMutex
	filePath string
	// map of provider -> map of modelID -> isDisabled
	disabled map[string]map[string]bool
}

func NewModelStore(dataDir string) (*ModelStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	fp := filepath.Join(dataDir, "disabled_models.json")
	ms := &ModelStore{
		filePath: fp,
		disabled: make(map[string]map[string]bool),
	}

	if err := ms.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load disabled models: %w", err)
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
