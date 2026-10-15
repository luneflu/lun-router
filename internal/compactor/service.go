package compactor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// Config is the persisted compactor state.
type Config struct {
	Enabled bool `json:"enabled"`
}

type Service struct {
	mu       sync.RWMutex
	filePath string
	config   Config

	totalRequests   atomic.Int64
	compressedReqs  atomic.Int64
	totalBytesIn    atomic.Int64
	totalBytesOut   atomic.Int64
	totalSavedBytes atomic.Int64

	recentMu   sync.Mutex
	recentHits []Hit
}

func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	// compactor.json is canonical; older installs are migrated once, see migrateLegacyConfig.
	fp := filepath.Join(dataDir, "compactor.json")
	migrateLegacyConfig(dataDir, fp)

	s := &Service{
		filePath: fp,
		config: Config{
			Enabled: true, // Default to true, matching 9router
		},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		log.Printf("[compact] warning: failed to load config %s: %v", fp, err)
	}

	return s, nil
}

// migrateLegacyConfig adopts a pre-rename rtk.json / token_saver.json once, then
// removes it so the old key cannot silently flip the setting back to a default.
func migrateLegacyConfig(dataDir, fp string) {
	if _, err := os.Stat(fp); err == nil {
		return
	}
	for _, name := range []string{"rtk.json", "token_saver.json"} {
		legacy := filepath.Join(dataDir, name)
		data, err := os.ReadFile(legacy)
		if err != nil {
			continue
		}
		var old struct {
			Enabled    *bool `json:"enabled"`
			RTKEnabled *bool `json:"rtk_enabled"`
		}
		_ = json.Unmarshal(data, &old)
		_ = os.Remove(legacy)

		enabled := true
		if old.Enabled != nil {
			enabled = *old.Enabled
		} else if old.RTKEnabled != nil {
			enabled = *old.RTKEnabled
		}
		if out, err := json.MarshalIndent(Config{Enabled: enabled}, "", "  "); err == nil {
			_ = os.WriteFile(fp, out, 0644)
		}
		return
	}
}

func (s *Service) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
	return nil
}

func (s *Service) saveLocked() error {
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func (s *Service) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Enabled
}

func (s *Service) SetEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Enabled = enabled
	return s.saveLocked()
}

func (s *Service) ProcessBody(body map[string]any, bypass bool) *Stats {
	s.totalRequests.Add(1)

	if bypass || !s.Enabled() {
		return nil
	}

	stats := CompressMessages(body, true)
	if stats != nil && len(stats.Hits) > 0 {
		saved := stats.BytesBefore - stats.BytesAfter
		s.compressedReqs.Add(1)
		s.totalBytesIn.Add(int64(stats.BytesBefore))
		s.totalBytesOut.Add(int64(stats.BytesAfter))
		s.totalSavedBytes.Add(int64(saved))

		s.recentMu.Lock()
		s.recentHits = append(s.recentHits, stats.Hits...)
		if len(s.recentHits) > 20 {
			s.recentHits = s.recentHits[len(s.recentHits)-20:]
		}
		s.recentMu.Unlock()

		if logLine := stats.LogLine(); logLine != "" {
			log.Println(logLine)
		}
	}
	return stats
}

func (s *Service) TestCompress(text string) map[string]any {
	bytesIn := len(text)

	out, filterName := compressOne(text)
	var filter any
	if filterName != "" {
		filter = filterName
	} else {
		out = text
	}

	bytesOut := len(out)
	saved := bytesIn - bytesOut
	savedPct := 0.0
	if bytesIn > 0 && saved > 0 {
		savedPct = (float64(saved) / float64(bytesIn)) * 100.0
	}

	return map[string]any{
		"filter":          filter,
		"original_size":   bytesIn,
		"compressed_size": bytesOut,
		"saved_bytes":     saved,
		"saved_percent":   savedPct,
		"output":          out,
	}
}

func (s *Service) GetStatus() map[string]any {
	s.mu.RLock()
	enabled := s.config.Enabled
	s.mu.RUnlock()

	bytesIn := s.totalBytesIn.Load()
	bytesOut := s.totalBytesOut.Load()
	savedBytes := s.totalSavedBytes.Load()
	pct := 0.0
	if bytesIn > 0 {
		pct = (float64(savedBytes) / float64(bytesIn)) * 100.0
	}

	s.recentMu.Lock()
	recent := make([]Hit, len(s.recentHits))
	copy(recent, s.recentHits)
	s.recentMu.Unlock()

	return map[string]any{
		"enabled": enabled,
		"engine":  "embedded-go",
		"stats": map[string]any{
			"total_requests":      s.totalRequests.Load(),
			"compressed_requests": s.compressedReqs.Load(),
			"bytes_before":        bytesIn,
			"bytes_after":         bytesOut,
			"saved_bytes":         savedBytes,
			"saved_percent":       pct,
			"recent_hits":         recent,
		},
		"supported_filters": []string{
			"json",
			"git-diff",
			"git-status",
			"git-log",
			"grep",
			"find",
			"ls",
			"tree",
			"build-output",
			"search-list",
			"read-numbered",
			"cargo-test",
			"pytest",
			"go-test",
			"dedup-log",
			"smart-truncate",
		},
	}
}
