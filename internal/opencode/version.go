package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	DefaultVersion = "2.0.26"
	VersionAPI     = "https://api.github.com/repos/anomalyco/opencode/releases/latest"
)

var semverRegex = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)

type VersionCache struct {
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type VersionManager struct {
	mu            sync.RWMutex
	version       string
	cacheFilePath string
	checkInterval time.Duration
	httpClient    *http.Client
}

func NewVersionManager(dataDir string, interval time.Duration) *VersionManager {
	if interval <= 0 {
		interval = 2 * time.Hour
	}

	var cachePath string
	if dataDir != "" {
		cachePath = filepath.Join(dataDir, "opencode_version.json")
	}

	vm := &VersionManager{
		version:       DefaultVersion,
		cacheFilePath: cachePath,
		checkInterval: interval,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	vm.loadCache()
	return vm
}

func (vm *VersionManager) loadCache() {
	if vm.cacheFilePath == "" {
		return
	}
	data, err := os.ReadFile(vm.cacheFilePath)
	if err != nil {
		return
	}
	var cached VersionCache
	if err := json.Unmarshal(data, &cached); err == nil && cached.Version != "" {
		vm.version = cached.Version
	}
}

func (vm *VersionManager) saveCache(v string) {
	if vm.cacheFilePath == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(vm.cacheFilePath), 0755)
	cached := VersionCache{
		Version:   v,
		UpdatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(vm.cacheFilePath, data, 0644)
}

func (vm *VersionManager) GetVersion() string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.version
}

func (vm *VersionManager) GetUserAgent() string {
	return fmt.Sprintf("opencode/latest/%s/cli", vm.GetVersion())
}

func (vm *VersionManager) SetVersion(v string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if !semverRegex.MatchString(v) {
		return
	}
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.version = v
	vm.saveCache(v)
}

// FetchLatestVersion queries the public GitHub release API
func (vm *VersionManager) FetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", VersionAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "lun-router/"+vm.GetVersion())
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := vm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("version check request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api status: %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decode release json: %w", err)
	}

	raw := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if !semverRegex.MatchString(raw) {
		return "", fmt.Errorf("invalid semver returned: %q", raw)
	}

	return raw, nil
}

// CheckAndUpdate checks GitHub for a newer version and updates in-memory + file cache
func (vm *VersionManager) CheckAndUpdate(ctx context.Context) (string, bool, error) {
	latest, err := vm.FetchLatestVersion(ctx)
	if err != nil {
		return vm.GetVersion(), false, err
	}

	vm.saveCache(latest)
	current := vm.GetVersion()
	if latest != current {
		vm.SetVersion(latest)
		log.Printf("[opencode] Detected new upstream OpenCode version: %s (was %s)", latest, current)
		return latest, true, nil
	}

	return current, false, nil
}

// StartBackgroundSync starts the periodic version checker job
func (vm *VersionManager) StartBackgroundSync(ctx context.Context) {
	// Initial check on boot
	go func() {
		checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		_, _, _ = vm.CheckAndUpdate(checkCtx)
	}()

	// Periodic ticker
	go func() {
		ticker := time.NewTicker(vm.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				_, _, _ = vm.CheckAndUpdate(checkCtx)
				cancel()
			}
		}
	}()
}
