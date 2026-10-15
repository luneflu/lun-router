package rtk

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type TokenSaverConfig struct {
	RTKEnabled bool `json:"rtk_enabled"`
}

type Service struct {
	mu       sync.RWMutex
	filePath string
	config   TokenSaverConfig

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

	fp := filepath.Join(dataDir, "token_saver.json")
	s := &Service{
		filePath: fp,
		config: TokenSaverConfig{
			RTKEnabled: true, // Default to true, matching 9router
		},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		log.Printf("[RTK] warning: failed to load config %s: %v", fp, err)
	}

	return s, nil
}

func (s *Service) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var cfg TokenSaverConfig
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

func (s *Service) IsRTKEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.RTKEnabled
}

func (s *Service) SetRTKEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.RTKEnabled = enabled
	return s.saveLocked()
}

func (s *Service) ProcessBody(body map[string]any, bypass bool) *Stats {
	s.totalRequests.Add(1)

	if bypass || !s.IsRTKEnabled() {
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
	fn, filterName := AutoDetectFilter(text)
	if fn == nil {
		return map[string]any{
			"filter":        nil,
			"original_size": bytesIn,
			"compressed_size": bytesIn,
			"saved_bytes":   0,
			"saved_percent": 0.0,
			"output":        text,
		}
	}

	out := SafeApply(fn, text)
	bytesOut := len(out)
	saved := bytesIn - bytesOut
	savedPct := 0.0
	if bytesIn > 0 && saved > 0 {
		savedPct = (float64(saved) / float64(bytesIn)) * 100.0
	}

	return map[string]any{
		"filter":          filterName,
		"original_size":   bytesIn,
		"compressed_size": bytesOut,
		"saved_bytes":     saved,
		"saved_percent":   savedPct,
		"output":          out,
	}
}

func (s *Service) GetStatus() map[string]any {
	s.mu.RLock()
	enabled := s.config.RTKEnabled
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
		"rtk_enabled": enabled,
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
			"dedup-log",
			"smart-truncate",
		},
	}
}
