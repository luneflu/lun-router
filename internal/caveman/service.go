package caveman

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
		filePath: filepath.Join(dataDir, "caveman.json"),
		config: Config{
			Enabled: true,
			Mode:    "caveman",
		},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		log.Printf("[Caveman] warning: failed to load config %s: %v", s.filePath, err)
	}

	s.mu.Lock()
	if builtinPrompt(s.config.Mode) == "" && s.config.Mode != "custom" {
		s.config.Mode = "caveman"
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
			log.Printf("[Caveman] warning: failed to drop legacy config keys: %v", err)
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
	case "caveman":
		return DefaultCavemanPrompt
	case "ultracave":
		return DefaultUltracavePrompt
	case "megacave":
		return DefaultMegacavePrompt
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
	return DefaultCavemanPrompt
}

var availableModes = []ModeInfo{
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
			TotalRequests:     s.totalRequests.Load(),
			InjectedRequests:  s.injectedRequests.Load(),
			EstimatedSavedPct: 54, // Average 54% reduction per benchmark
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
	custom := s.config.CustomPrompt
	s.mu.RUnlock()

	if !enabled && headerCaveman != "on" && headerCaveman != "true" && headerMode == "" {
		return false
	}

	if headerMode != "" {
		hm := strings.ToLower(strings.TrimSpace(headerMode))
		if builtinPrompt(hm) != "" || hm == "custom" {
			mode = hm
		}
	} else if headerCaveman != "" && headerCaveman != "on" && headerCaveman != "true" {
		if headerCaveman == "ultracave" || headerCaveman == "ultra" {
			mode = "ultracave"
		} else if headerCaveman == "megacave" || headerCaveman == "wenyan" {
			mode = "megacave"
		}
	}

	prompt := activePrompt(mode, custom)
	cavemanInstruction := "\n\n[System directive: Response style]\n" + prompt
	reminder := shortCavemanReminder(mode, custom)

	// 1. Anthropic-style top-level "system"
	if rawSys, ok := body["system"]; ok {
		if sysStr, ok := rawSys.(string); ok && sysStr != "" {
			body["system"] = sysStr + cavemanInstruction
			s.injectedRequests.Add(1)
			if rawMsgs, ok := body["messages"]; ok {
				if msgs, ok := rawMsgs.([]any); ok && shouldAnchorTail(msgs) {
					if appendTailReminder(msgs, reminder) {
						body["messages"] = msgs
					}
				}
			}
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
		// Prepend system message. Same instruction text as the append path, header
		// included, so the injected directive does not depend on the body shape.
		newSys := map[string]any{
			"role":    "system",
			"content": cavemanInstruction,
		}
		msgs = append([]any{newSys}, msgs...)
	}

	// Tail reinforcement: anchor reminder at the last user message when context is long or multi-turn
	if shouldAnchorTail(msgs) {
		appendTailReminder(msgs, reminder)
	}

	body["messages"] = msgs
	s.injectedRequests.Add(1)
	return true
}

func shortCavemanReminder(mode, custom string) string {
	switch mode {
	case "ultracave":
		return "[System directive reminder: Ultracave mode active. Internal thinking and response: pure shorthand fragments, dense technical notes, zero fluff. Match user language for thinking. Technical payload verbatim.]"
	case "megacave":
		return "[System directive reminder: Megacave (文言文) active. 內部思考（thinking）與回覆皆純以文言文深思，去贅言。代碼命令精確保留。]"
	case "custom":
		firstLine := strings.TrimSpace(strings.Split(custom, "\n")[0])
		if len(firstLine) > 100 {
			firstLine = firstLine[:100] + "..."
		}
		if firstLine != "" {
			return fmt.Sprintf("[System directive reminder: %s]", firstLine)
		}
		return "[System directive reminder: Caveman mode active. Internal thinking and response: terse shorthand, direct, zero fluff. Match user language. Technical payload verbatim.]"
	default:
		return "[System directive reminder: Caveman mode active. Internal thinking and response: terse shorthand, direct, zero fluff, kill ceremony. Match user language. Technical payload verbatim.]"
	}
}

func shouldAnchorTail(msgs []any) bool {
	hasAssistant := false
	nonSystemCount := 0
	totalChars := 0

	for _, m := range msgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role == "assistant" {
			hasAssistant = true
		}
		if role == "system" {
			continue
		}
		nonSystemCount++
		switch c := msgMap["content"].(type) {
		case string:
			totalChars += len(c)
		case []any:
			for _, part := range c {
				if partMap, ok := part.(map[string]any); ok {
					if txt, ok := partMap["text"].(string); ok {
						totalChars += len(txt)
					}
				}
			}
		}
	}

	// Anchor if prior conversation exists, or non-system context is substantial (> 2000 chars)
	return hasAssistant || nonSystemCount >= 2 || totalChars >= 2000
}

func appendTailReminder(msgs []any, reminder string) bool {
	for i := len(msgs) - 1; i >= 0; i-- {
		msgMap, ok := msgs[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role != "user" {
			continue
		}

		switch content := msgMap["content"].(type) {
		case string:
			if strings.Contains(content, "[System directive reminder:") {
				// If a reminder already exists, check if this specific reminder is in it
				if strings.Contains(content, reminder) {
					return false
				}
				// Otherwise append below the previous reminder
				msgMap["content"] = content + "\n" + reminder
				msgs[i] = msgMap
				return true
			}
			msgMap["content"] = content + "\n\n" + reminder
			msgs[i] = msgMap
			return true
		case []any:
			for _, part := range content {
				if partMap, ok := part.(map[string]any); ok {
					if txt, ok := partMap["text"].(string); ok && strings.Contains(txt, reminder) {
						return false
					}
				}
			}
			appended := false
			for j := len(content) - 1; j >= 0; j-- {
				if part, ok := content[j].(map[string]any); ok {
					if partType, ok := part["type"].(string); ok && partType == "text" {
						if txt, ok := part["text"].(string); ok {
							if strings.Contains(txt, "[System directive reminder:") {
								part["text"] = txt + "\n" + reminder
							} else {
								part["text"] = txt + "\n\n" + reminder
							}
							appended = true
							break
						}
					}
				}
			}
			if !appended {
				content = append(content, map[string]any{
					"type": "text",
					"text": reminder,
				})
				msgMap["content"] = content
			}
			msgs[i] = msgMap
			return true
		}
	}
	return false
}
