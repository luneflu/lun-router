package caveman

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	UpstreamRepo       = "https://github.com/JuliusBrussee/caveman"
	GitHubCommitAPI    = "https://api.github.com/repos/JuliusBrussee/caveman/commits/main"
	DefaultCheckPeriod = 6 * time.Hour
)

type Service struct {
	mu            sync.RWMutex
	filePath      string
	config        Config
	httpClient    *http.Client
	stopCh        chan struct{}

	totalRequests    atomic.Int64
	injectedRequests atomic.Int64
}

func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	fp := filepath.Join(dataDir, "caveman.json")
	s := &Service{
		filePath: fp,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopCh: make(chan struct{}),
		config: Config{
			Enabled:            true,
			Mode:               "caveman",
			AutoUpdate:         true,
			CheckIntervalHours: 6,
			Rules: map[string]string{
				"caveman":   DefaultCavemanPrompt,
				"ultracave": DefaultUltracavePrompt,
				"megacave":  DefaultMegacavePrompt,
			},
		},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		log.Printf("[Caveman] warning: failed to load config %s: %v", fp, err)
	}

	// Ensure rules are not empty
	s.mu.Lock()
	if s.config.Rules == nil {
		s.config.Rules = make(map[string]string)
	}
	if s.config.Rules["caveman"] == "" {
		s.config.Rules["caveman"] = DefaultCavemanPrompt
	}
	if s.config.Rules["ultracave"] == "" {
		s.config.Rules["ultracave"] = DefaultUltracavePrompt
	}
	if s.config.Rules["megacave"] == "" {
		s.config.Rules["megacave"] = DefaultMegacavePrompt
	}
	if s.config.Mode == "" {
		s.config.Mode = "caveman"
	}
	if s.config.CheckIntervalHours <= 0 {
		s.config.CheckIntervalHours = 6
	}
	s.mu.Unlock()

	// Initial async sync check and background timer
	go s.startBackgroundWorker()

	return s, nil
}

func (s *Service) Close() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
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
	return os.WriteFile(s.filePath, data, 0644)
}

func (s *Service) startBackgroundWorker() {
	// Wait 2 seconds after startup for initial check
	select {
	case <-s.stopCh:
		return
	case <-time.After(2 * time.Second):
	}

	s.mu.RLock()
	autoUpdate := s.config.AutoUpdate
	hours := s.config.CheckIntervalHours
	s.mu.RUnlock()

	if autoUpdate {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		updated, err := s.SyncUpstream(ctx)
		cancel()
		if err != nil {
			log.Printf("[Caveman] initial upstream sync check failed: %v", err)
		} else if updated {
			log.Printf("[Caveman] initial upstream sync: updated rules to latest commit")
		}
	}

	interval := time.Duration(hours) * time.Hour
	if interval < time.Hour {
		interval = DefaultCheckPeriod
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.RLock()
			auto := s.config.AutoUpdate
			s.mu.RUnlock()

			if auto {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, _ = s.SyncUpstream(ctx)
				cancel()
			}
		}
	}
}

// cleanSkillMarkdown strips YAML frontmatter and extracts prompt body
func cleanSkillMarkdown(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.HasPrefix(raw, "---\n") {
		idx := strings.Index(raw[4:], "\n---\n")
		if idx != -1 {
			raw = raw[4+idx+5:]
		}
	}
	return strings.TrimSpace(raw)
}

// SyncUpstream checks GitHub for changes and updates rules
func (s *Service) SyncUpstream(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", GitHubCommitAPI, nil)
	if err != nil {
		return false, fmt.Errorf("create req: %w", err)
	}
	req.Header.Set("User-Agent", "lun-router-caveman/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetch commit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("github api status %d: %s", resp.StatusCode, string(body))
	}

	var commitResp struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&commitResp); err != nil {
		return false, fmt.Errorf("decode commit json: %w", err)
	}

	s.mu.RLock()
	currentSHA := s.config.UpstreamCommit
	s.mu.RUnlock()

	// If commit hasn't changed, just touch LastSync
	if currentSHA == commitResp.SHA && currentSHA != "" {
		s.mu.Lock()
		s.config.LastSync = time.Now()
		_ = s.saveLocked()
		s.mu.Unlock()
		return false, nil
	}

	// Fetch latest skill files from raw GitHub
	fetchSkill := func(skillName string) (string, error) {
		u := fmt.Sprintf("https://raw.githubusercontent.com/JuliusBrussee/caveman/main/skills/%s/SKILL.md", skillName)
		r, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return "", err
		}
		r.Header.Set("User-Agent", "lun-router-caveman/1.0")
		res, err := s.httpClient.Do(r)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return "", fmt.Errorf("status %d for %s", res.StatusCode, skillName)
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		if err != nil {
			return "", err
		}
		cleaned := cleanSkillMarkdown(string(b))
		if len(cleaned) == 0 {
			return "", fmt.Errorf("empty skill body for %s", skillName)
		}
		return cleaned, nil
	}

	newRules := make(map[string]string)
	for _, skill := range []string{"caveman", "ultracave", "megacave"} {
		content, err := fetchSkill(skill)
		if err != nil {
			log.Printf("[Caveman] warning: failed to fetch skill %s: %v (using fallback)", skill, err)
			s.mu.RLock()
			newRules[skill] = s.config.Rules[skill]
			s.mu.RUnlock()
		} else {
			newRules[skill] = content
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range newRules {
		if v != "" {
			s.config.Rules[k] = v
		}
	}
	s.config.UpstreamCommit = commitResp.SHA
	s.config.UpstreamCommitMsg = strings.TrimSpace(strings.Split(commitResp.Commit.Message, "\n")[0])
	s.config.UpstreamCommitDate = commitResp.Commit.Author.Date
	s.config.LastSync = time.Now()

	if err := s.saveLocked(); err != nil {
		log.Printf("[Caveman] failed to save updated config: %v", err)
	}

	log.Printf("[Caveman] synced rules with upstream commit %s (%s)", commitResp.SHA[:8], s.config.UpstreamCommitMsg)
	return true, nil
}

// GetStatus returns current operational state
func (s *Service) GetStatus() StatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	activePrompt := s.config.Rules[s.config.Mode]
	if s.config.Mode == "custom" && s.config.CustomPrompt != "" {
		activePrompt = s.config.CustomPrompt
	}

	var lastSyncPtr *time.Time
	if !s.config.LastSync.IsZero() {
		ls := s.config.LastSync
		lastSyncPtr = &ls
	}

	shortCommit := s.config.UpstreamCommit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}

	available := []ModeInfo{
		{
			ID:          "caveman",
			Name:        "Classic Caveman",
			Description: "Answer first, no fluff, every technical fact preserved. Standard English.",
			TokensEst:   20,
		},
		{
			ID:          "ultracave",
			Name:        "Ultracave",
			Description: "Grammar stripped, fragments only, maximum English terseness.",
			TokensEst:   14,
		},
		{
			ID:          "megacave",
			Name:        "Megacave (文言文)",
			Description: "Classical Chinese register, subjects omitted, maximum character reduction.",
			TokensEst:   13,
		},
		{
			ID:          "custom",
			Name:        "Custom Rules",
			Description: "User-defined system compression prompt instructions.",
			TokensEst:   0,
		},
	}

	return StatusResponse{
		Enabled:            s.config.Enabled,
		Mode:               s.config.Mode,
		AutoUpdate:         s.config.AutoUpdate,
		LastSync:           lastSyncPtr,
		UpstreamCommit:     shortCommit,
		UpstreamCommitMsg:  s.config.UpstreamCommitMsg,
		UpstreamCommitDate: s.config.UpstreamCommitDate,
		UpstreamURL:        UpstreamRepo,
		ActivePrompt:       activePrompt,
		AvailableModes:     available,
		CustomPrompt:       s.config.CustomPrompt,
		Stats: Stats{
			TotalRequests:     s.totalRequests.Load(),
			InjectedRequests:  s.injectedRequests.Load(),
			EstimatedSavedPct: 54, // Average 54% reduction per benchmark
		},
	}
}

// UpdateConfig updates settings
func (s *Service) UpdateConfig(enabled *bool, mode *string, autoUpdate *bool, customPrompt *string) (StatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if enabled != nil {
		s.config.Enabled = *enabled
	}
	if mode != nil && *mode != "" {
		m := strings.ToLower(strings.TrimSpace(*mode))
		if m == "caveman" || m == "ultracave" || m == "megacave" || m == "custom" {
			s.config.Mode = m
		}
	}
	if autoUpdate != nil {
		s.config.AutoUpdate = *autoUpdate
	}
	if customPrompt != nil {
		s.config.CustomPrompt = *customPrompt
	}

	if err := s.saveLocked(); err != nil {
		return StatusResponse{}, fmt.Errorf("save config: %w", err)
	}

	s.mu.Unlock()
	st := s.GetStatus()
	s.mu.Lock()
	return st, nil
}

// Apply injects Caveman prompt into completion messages payload
func (s *Service) Apply(body map[string]any, headerCaveman, headerMode string) bool {
	s.totalRequests.Add(1)

	headerCaveman = strings.ToLower(strings.TrimSpace(headerCaveman))
	if headerCaveman == "off" || headerCaveman == "false" || headerCaveman == "0" {
		return false
	}

	s.mu.RLock()
	enabled := s.config.Enabled
	mode := s.config.Mode
	rules := s.config.Rules
	custom := s.config.CustomPrompt
	s.mu.RUnlock()

	if !enabled && headerCaveman != "on" && headerCaveman != "true" && headerMode == "" {
		return false
	}

	if headerMode != "" {
		hm := strings.ToLower(strings.TrimSpace(headerMode))
		if hm == "caveman" || hm == "ultracave" || hm == "megacave" || hm == "custom" {
			mode = hm
		}
	} else if headerCaveman != "" && headerCaveman != "on" && headerCaveman != "true" {
		if headerCaveman == "ultracave" || headerCaveman == "ultra" {
			mode = "ultracave"
		} else if headerCaveman == "megacave" || headerCaveman == "wenyan" {
			mode = "megacave"
		}
	}

	prompt := rules[mode]
	if mode == "custom" && custom != "" {
		prompt = custom
	}
	if prompt == "" {
		prompt = DefaultCavemanPrompt
	}

	cavemanInstruction := "\n\n[System directive: Response style]\n" + prompt

	// 1. Anthropic-style top-level "system"
	if rawSys, ok := body["system"]; ok {
		if sysStr, ok := rawSys.(string); ok && sysStr != "" {
			body["system"] = sysStr + cavemanInstruction
			s.injectedRequests.Add(1)
			return true
		}
	}

	// 2. OpenAI-style "messages"
	rawMsgs, ok := body["messages"]
	if !ok {
		return false
	}
	msgs, ok := rawMsgs.([]any)
	if !ok {
		return false
	}

	// Find existing system message
	foundSystem := false
	for i, m := range msgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if role, ok := msgMap["role"].(string); ok && role == "system" {
			foundSystem = true
			if content, ok := msgMap["content"].(string); ok {
				msgMap["content"] = content + cavemanInstruction
			} else {
				msgMap["content"] = cavemanInstruction
			}
			msgs[i] = msgMap
			break
		}
	}

	if !foundSystem {
		// Prepend system message
		newSys := map[string]any{
			"role":    "system",
			"content": prompt,
		}
		msgs = append([]any{newSys}, msgs...)
	}

	body["messages"] = msgs
	s.injectedRequests.Add(1)
	return true
}
