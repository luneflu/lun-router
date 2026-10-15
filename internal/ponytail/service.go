package ponytail

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
	UpstreamRepo       = "https://github.com/DietrichGebert/ponytail"
	GitHubCommitAPI    = "https://api.github.com/repos/DietrichGebert/ponytail/commits/main"
	DefaultCheckPeriod = 6 * time.Hour
)

type Service struct {
	mu         sync.RWMutex
	filePath   string
	config     Config
	httpClient *http.Client
	stopCh     chan struct{}

	totalRequests    atomic.Int64
	injectedRequests atomic.Int64
}

func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	fp := filepath.Join(dataDir, "ponytail.json")
	s := &Service{
		filePath: fp,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopCh: make(chan struct{}),
		config: Config{
			Enabled:            true,
			Mode:               "full",
			AutoUpdate:         true,
			CheckIntervalHours: 6,
			Rules: map[string]string{
				"full":   DefaultPonytailFullPrompt,
				"lite":   DefaultPonytailLitePrompt,
				"ultra":  DefaultPonytailUltraPrompt,
				"review": DefaultPonytailReviewPrompt,
			},
		},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		log.Printf("[Ponytail] warning: failed to load config %s: %v", fp, err)
	}

	s.mu.Lock()
	if s.config.Rules == nil {
		s.config.Rules = make(map[string]string)
	}
	if s.config.Rules["full"] == "" {
		s.config.Rules["full"] = DefaultPonytailFullPrompt
	}
	if s.config.Rules["lite"] == "" {
		s.config.Rules["lite"] = DefaultPonytailLitePrompt
	}
	if s.config.Rules["ultra"] == "" {
		s.config.Rules["ultra"] = DefaultPonytailUltraPrompt
	}
	if s.config.Rules["review"] == "" {
		s.config.Rules["review"] = DefaultPonytailReviewPrompt
	}
	if s.config.Mode == "" {
		s.config.Mode = "full"
	}
	if s.config.CheckIntervalHours <= 0 {
		s.config.CheckIntervalHours = 6
	}
	s.mu.Unlock()

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
			log.Printf("[Ponytail] initial upstream sync check failed: %v", err)
		} else if updated {
			log.Printf("[Ponytail] initial upstream sync: updated rules to latest commit")
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
	req.Header.Set("User-Agent", "lun-router-ponytail/1.0")
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

	if currentSHA == commitResp.SHA && currentSHA != "" {
		s.mu.Lock()
		s.config.LastSync = time.Now()
		_ = s.saveLocked()
		s.mu.Unlock()
		return false, nil
	}

	fetchSkill := func(url string) (string, error) {
		r, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return "", err
		}
		r.Header.Set("User-Agent", "lun-router-ponytail/1.0")
		res, err := s.httpClient.Do(r)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return "", fmt.Errorf("status %d", res.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		if err != nil {
			return "", err
		}
		cleaned := cleanSkillMarkdown(string(b))
		if len(cleaned) == 0 {
			return "", fmt.Errorf("empty skill body")
		}
		return cleaned, nil
	}

	newRules := make(map[string]string)

	mainSkill, err := fetchSkill("https://raw.githubusercontent.com/DietrichGebert/ponytail/main/skills/ponytail/SKILL.md")
	if err == nil && len(mainSkill) > 0 {
		newRules["full"] = mainSkill
		newRules["lite"] = DefaultPonytailLitePrompt
		newRules["ultra"] = DefaultPonytailUltraPrompt
	}

	reviewSkill, err := fetchSkill("https://raw.githubusercontent.com/DietrichGebert/ponytail/main/skills/ponytail-review/SKILL.md")
	if err == nil && len(reviewSkill) > 0 {
		newRules["review"] = reviewSkill
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
		log.Printf("[Ponytail] failed to save updated config: %v", err)
	}

	log.Printf("[Ponytail] synced rules with upstream commit %s (%s)", commitResp.SHA[:8], s.config.UpstreamCommitMsg)
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
			ID:          "full",
			Name:        "Full (Default)",
			Description: "The smallest complete change that fully works. YAGNI, standard library first, zero unasked abstractions.",
		},
		{
			ID:          "lite",
			Name:        "Lite",
			Description: "Builds what was requested, but names the smaller lazy option in one line for the user to choose.",
		},
		{
			ID:          "ultra",
			Name:        "Ultra (Pushback)",
			Description: "Questions requests and pushes back on speculative complexity before writing a line of code.",
		},
		{
			ID:          "review",
			Name:        "Review",
			Description: "Senior developer code review: correctness, safety, load resilience, lean diffs, and zero bloat.",
		},
		{
			ID:          "custom",
			Name:        "Custom Rules",
			Description: "Custom user-defined senior developer guidelines.",
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
			TotalRequests:    s.totalRequests.Load(),
			InjectedRequests: s.injectedRequests.Load(),
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
		if m == "full" || m == "lite" || m == "ultra" || m == "review" || m == "custom" {
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

// Apply injects Ponytail instructions into completion messages payload
func (s *Service) Apply(body map[string]any, headerPonytail, headerMode string) bool {
	s.totalRequests.Add(1)

	headerPonytail = strings.ToLower(strings.TrimSpace(headerPonytail))
	if headerPonytail == "off" || headerPonytail == "false" || headerPonytail == "0" {
		return false
	}

	s.mu.RLock()
	enabled := s.config.Enabled
	mode := s.config.Mode
	rules := s.config.Rules
	custom := s.config.CustomPrompt
	s.mu.RUnlock()

	if !enabled && headerPonytail != "on" && headerPonytail != "true" && headerMode == "" {
		return false
	}

	if headerMode != "" {
		hm := strings.ToLower(strings.TrimSpace(headerMode))
		if hm == "full" || hm == "lite" || hm == "ultra" || hm == "review" || hm == "custom" {
			mode = hm
		}
	} else if headerPonytail != "" && headerPonytail != "on" && headerPonytail != "true" {
		if headerPonytail == "lite" || headerPonytail == "ultra" || headerPonytail == "review" {
			mode = headerPonytail
		}
	}

	prompt := rules[mode]
	if mode == "custom" && custom != "" {
		prompt = custom
	}
	if prompt == "" {
		prompt = DefaultPonytailFullPrompt
	}

	ponytailInstruction := "\n\n[System directive: Lazy Senior Developer Guidance (" + mode + ")]\n" + prompt + "\nStandalone / testing queries: When no project repository or workspace directory is attached, do not scan or ask for files. Provide a small, self-contained runnable example directly."

	// 1. Anthropic-style top-level "system"
	if rawSys, ok := body["system"]; ok {
		if sysStr, ok := rawSys.(string); ok && sysStr != "" {
			body["system"] = sysStr + ponytailInstruction
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

	foundSystem := false
	for i, m := range msgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if role, ok := msgMap["role"].(string); ok && role == "system" {
			foundSystem = true
			if content, ok := msgMap["content"].(string); ok {
				msgMap["content"] = content + ponytailInstruction
			} else {
				msgMap["content"] = ponytailInstruction
			}
			msgs[i] = msgMap
			break
		}
	}

	if !foundSystem {
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
