package ponytail

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// standaloneTail is appended to every mode. Ponytail's rules assume a project repo is
// attached; on a bare API call there is nothing to scan, so say so.
const standaloneTail = "\nStandalone / testing queries: When no project repository or workspace directory is attached, do not scan or ask for files. Provide a small, self-contained runnable example directly."

type Service struct {
	mu       sync.RWMutex
	filePath string
	config   Config

	totalRequests    atomic.Int64
	injectedRequests atomic.Int64
}

func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	s := &Service{
		filePath: filepath.Join(dataDir, "ponytail.json"),
		config: Config{
			Enabled: true,
			Mode:    "full",
		},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		log.Printf("[Ponytail] warning: failed to load config %s: %v", s.filePath, err)
	}

	s.mu.Lock()
	if builtinPrompt(s.config.Mode) == "" && s.config.Mode != "custom" {
		s.config.Mode = "full"
	}
	s.mu.Unlock()

	return s, nil
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

	// A data dir written before the rules moved into the binary still carries the
	// prompt text and the upstream-sync settings. Unknown keys are ignored on decode,
	// so rewrite the file once to drop them; nothing reads them anymore.
	var legacy struct {
		Rules      map[string]string `json:"rules"`
		AutoUpdate *bool             `json:"auto_update"`
	}
	_ = json.Unmarshal(data, &legacy)
	prune := legacy.Rules != nil || legacy.AutoUpdate != nil

	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
	if prune {
		if err := s.saveLocked(); err != nil {
			log.Printf("[Ponytail] warning: failed to drop legacy config keys: %v", err)
		}
	}
	return nil
}

func (s *Service) saveLocked() error {
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// builtinPrompt returns the compiled-in rule text for a mode, or "" when the mode is
// not one of the built-ins.
func builtinPrompt(mode string) string {
	switch mode {
	case "full":
		return DefaultPonytailFullPrompt
	case "lite":
		return DefaultPonytailLitePrompt
	case "ultra":
		return DefaultPonytailUltraPrompt
	case "review":
		return DefaultPonytailReviewPrompt
	default:
		return ""
	}
}

// activePrompt resolves the text that gets injected. Built-in modes read straight from
// the constants in defaults.go, so a redeploy takes effect on the next restart; no
// stored copy and nothing fetched at runtime. Only the custom mode carries text from
// disk.
func activePrompt(mode, custom string) string {
	if mode == "custom" && custom != "" {
		return custom
	}
	if prompt := builtinPrompt(mode); prompt != "" {
		return prompt
	}
	return DefaultPonytailFullPrompt
}

var availableModes = []ModeInfo{
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

// GetStatus returns current operational state
func (s *Service) GetStatus() StatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return StatusResponse{
		Enabled:        s.config.Enabled,
		Mode:           s.config.Mode,
		ActivePrompt:   activePrompt(s.config.Mode, s.config.CustomPrompt),
		AvailableModes: availableModes,
		CustomPrompt:   s.config.CustomPrompt,
		Stats: Stats{
			TotalRequests:    s.totalRequests.Load(),
			InjectedRequests: s.injectedRequests.Load(),
		},
	}
}

// UpdateConfig updates settings
func (s *Service) UpdateConfig(enabled *bool, mode *string, customPrompt *string) (StatusResponse, error) {
	s.mu.Lock()

	if enabled != nil {
		s.config.Enabled = *enabled
	}
	if mode != nil && *mode != "" {
		m := strings.ToLower(strings.TrimSpace(*mode))
		if builtinPrompt(m) != "" || m == "custom" {
			s.config.Mode = m
		}
	}
	if customPrompt != nil {
		s.config.CustomPrompt = *customPrompt
	}

	err := s.saveLocked()
	s.mu.Unlock()

	if err != nil {
		return StatusResponse{}, fmt.Errorf("save config: %w", err)
	}
	return s.GetStatus(), nil
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
	custom := s.config.CustomPrompt
	s.mu.RUnlock()

	if !enabled && headerPonytail != "on" && headerPonytail != "true" && headerMode == "" {
		return false
	}

	if headerMode != "" {
		hm := strings.ToLower(strings.TrimSpace(headerMode))
		if builtinPrompt(hm) != "" || hm == "custom" {
			mode = hm
		}
	} else if headerPonytail != "" && headerPonytail != "on" && headerPonytail != "true" {
		if headerPonytail == "lite" || headerPonytail == "ultra" || headerPonytail == "review" {
			mode = headerPonytail
		}
	}

	prompt := activePrompt(mode, custom)

	ponytailInstruction := "\n\n[System directive: Lazy Senior Developer Guidance (" + mode + ")]\n" + prompt + standaloneTail

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
				msgMap["content"] = content + ponytailInstruction
			} else {
				msgMap["content"] = ponytailInstruction
			}
			msgs[i] = msgMap
			break
		}
	}

	if !foundSystem {
		// Prepend system message
		newSys := map[string]any{
			"role":    "system",
			"content": ponytailInstruction,
		}
		msgs = append([]any{newSys}, msgs...)
	}

	body["messages"] = msgs
	s.injectedRequests.Add(1)
	return true
}
