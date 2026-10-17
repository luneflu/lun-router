package model

import (
	"path/filepath"
	"strings"
)

// ModelCapabilities represents capabilities and thinking specifications for a model.
type ModelCapabilities struct {
	Vision             bool     `json:"vision,omitempty"`
	Tools              bool     `json:"tools"`
	Reasoning          bool     `json:"reasoning"`
	ThinkingFormat     string   `json:"thinking_format,omitempty"`
	ThinkingLevels     []string `json:"thinking_levels,omitempty"`
	ThinkingCanDisable bool     `json:"thinking_can_disable"`
	ContextWindow      int      `json:"context_window,omitempty"`
	MaxOutput          int      `json:"max_output,omitempty"`
}

// ModelItem represents an OpenAI-compatible model object with capabilities metadata.
type ModelItem struct {
	ID                  string             `json:"id"`
	Object              string             `json:"object"`
	OwnedBy             string             `json:"owned_by"`
	Enabled             bool               `json:"enabled"`
	Capabilities        *ModelCapabilities `json:"capabilities,omitempty"`
	ThinkingLevels      []string           `json:"thinking_levels,omitempty"`
	ContextLength       int                `json:"context_length,omitempty"`
	MaxCompletionTokens int                `json:"max_completion_tokens,omitempty"`
}

// Standard thinking level sets
var (
	LevelsOpenAI    = []string{"none", "minimal", "low", "medium", "high", "xhigh"}
	LevelsCodex     = []string{"low", "medium", "high", "xhigh"}
	LevelsAdaptive  = []string{"none", "low", "medium", "high", "max"}
	LevelsBudget    = []string{"none", "low", "medium", "high", "xhigh", "max"}
	LevelsGemini    = []string{"minimal", "low", "medium", "high"}
	LevelsDeepSeek  = []string{"none", "high", "max"}
	LevelsDeepSeek4 = []string{"none", "low", "medium", "high", "xhigh", "max"}
	LevelsBase      = []string{"none", "low", "medium", "high"}
	LevelsOnOff     = []string{"none", "thinking"}
	LevelsMiMo      = []string{"none", "low", "medium", "high", "xhigh"}
	LevelsKimiFixed = []string{"low", "medium", "high", "max"}
)

type patternRule struct {
	pattern string
	caps    ModelCapabilities
}

// ordered pattern rules (specific first, general last)
var patternRules = []patternRule{
	// OpenCode Muse Spark
	{
		pattern: "*muse*spark*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      1048576,
			MaxOutput:          131072,
		},
	},

	// OpenAI GPT-6
	{
		pattern: "*gpt-6*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      272000,
			MaxOutput:          128000,
		},
	},

	// OpenAI GPT-5 Codex
	{
		pattern: "*gpt-5*codex*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsCodex,
			ThinkingCanDisable: false,
			ContextWindow:      400000,
			MaxOutput:          128000,
		},
	},

	// OpenAI GPT-5
	{
		pattern: "*gpt-5*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      400000,
			MaxOutput:          128000,
		},
	},

	// OpenAI o-series
	{
		pattern: "*o1*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      200000,
			MaxOutput:          100000,
		},
	},
	{
		pattern: "*o3*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      200000,
			MaxOutput:          100000,
		},
	},
	{
		pattern: "*o4*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      200000,
			MaxOutput:          100000,
		},
	},

	// Grok
	{
		pattern: "*grok-code*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      256000,
			MaxOutput:          64000,
		},
	},
	{
		pattern: "*grok-4*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "openai",
			ThinkingLevels:     LevelsOpenAI,
			ThinkingCanDisable: true,
			ContextWindow:      500000,
			MaxOutput:          64000,
		},
	},

	// Claude Adaptive (Opus 4.6/4.7/4.8/5, Sonnet 4.6/4.7/5)
	{
		pattern: "*claude*opus-5*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "claude-adaptive",
			ThinkingLevels:     LevelsAdaptive,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*claude*opus-4*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "claude-adaptive",
			ThinkingLevels:     LevelsAdaptive,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*claude*sonnet-5*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "claude-adaptive",
			ThinkingLevels:     LevelsAdaptive,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*claude*sonnet-4*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "claude-adaptive",
			ThinkingLevels:     LevelsAdaptive,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*claude*fable-5-1*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "claude-adaptive",
			ThinkingLevels:     LevelsKimiFixed,
			ThinkingCanDisable: false,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*claude*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "claude-budget",
			ThinkingLevels:     LevelsBudget,
			ThinkingCanDisable: true,
			ContextWindow:      200000,
			MaxOutput:          64000,
		},
	},

	// Gemini 3.x
	{
		pattern: "*gemini-3*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "gemini-level",
			ThinkingLevels:     LevelsGemini,
			ThinkingCanDisable: false,
			ContextWindow:      1048576,
			MaxOutput:          65536,
		},
	},

	// Gemini 2.5
	{
		pattern: "*gemini-2.5*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "gemini-budget",
			ThinkingLevels:     LevelsBase,
			ThinkingCanDisable: true,
			ContextWindow:      1048576,
			MaxOutput:          65536,
		},
	},

	// DeepSeek V4.x
	{
		pattern: "*deepseek-v4.*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "deepseek",
			ThinkingLevels:     LevelsDeepSeek4,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*deepseek-v4*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "deepseek",
			ThinkingLevels:     LevelsDeepSeek4,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*deepseek*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "deepseek",
			ThinkingLevels:     LevelsDeepSeek,
			ThinkingCanDisable: true,
			ContextWindow:      128000,
			MaxOutput:          64000,
		},
	},

	// Kimi
	{
		pattern: "*kimi*k3*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "kimi",
			ThinkingLevels:     LevelsKimiFixed,
			ThinkingCanDisable: false,
			ContextWindow:      1048576,
			MaxOutput:          131072,
		},
	},
	{
		pattern: "*kimi*code*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "kimi",
			ThinkingLevels:     LevelsKimiFixed,
			ThinkingCanDisable: false,
			ContextWindow:      262144,
			MaxOutput:          65536,
		},
	},
	{
		pattern: "*kimi*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "kimi",
			ThinkingLevels:     LevelsAdaptive,
			ThinkingCanDisable: true,
			ContextWindow:      262144,
			MaxOutput:          262144,
		},
	},

	// Xiaomi MiMo
	{
		pattern: "*mimo*v2.6*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "deepseek",
			ThinkingLevels:     LevelsMiMo,
			ThinkingCanDisable: true,
			ContextWindow:      1048576,
			MaxOutput:          131072,
		},
	},
	{
		pattern: "*mimo*v2.5*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "deepseek",
			ThinkingLevels:     LevelsMiMo,
			ThinkingCanDisable: true,
			ContextWindow:      1048576,
			MaxOutput:          131072,
		},
	},

	// MiniMax
	{
		pattern: "*minimax-m3*",
		caps: ModelCapabilities{
			Vision:             true,
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "minimax",
			ThinkingLevels:     LevelsOnOff,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          131072,
		},
	},
	{
		pattern: "*minimax*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "minimax",
			ThinkingLevels:     []string{"thinking"},
			ThinkingCanDisable: false,
			ContextWindow:      204800,
			MaxOutput:          131072,
		},
	},

	// GLM / Z.ai
	{
		pattern: "*glm-5.3*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "zai",
			ThinkingLevels:     LevelsBase,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*glm-5.2*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "zai",
			ThinkingLevels:     LevelsBase,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          128000,
		},
	},
	{
		pattern: "*glm*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "zai",
			ThinkingLevels:     LevelsOnOff,
			ThinkingCanDisable: true,
			ContextWindow:      200000,
			MaxOutput:          128000,
		},
	},

	// Qwen
	{
		pattern: "*qwen*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingFormat:     "qwen",
			ThinkingLevels:     LevelsBase,
			ThinkingCanDisable: true,
			ContextWindow:      1000000,
			MaxOutput:          65536,
		},
	},

	// Nemotron, Step, Hunyuan
	{
		pattern: "*nemotron*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingLevels:     LevelsBase,
			ThinkingCanDisable: true,
			ContextWindow:      128000,
			MaxOutput:          64000,
		},
	},
	{
		pattern: "*step-*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingLevels:     LevelsBase,
			ThinkingCanDisable: true,
			ContextWindow:      128000,
			MaxOutput:          64000,
		},
	},
	{
		pattern: "*hunyuan*",
		caps: ModelCapabilities{
			Tools:              true,
			Reasoning:          true,
			ThinkingLevels:     LevelsBase,
			ThinkingCanDisable: true,
			ContextWindow:      262144,
			MaxOutput:          64000,
		},
	},
}

// matchesPattern checks if candidate matches wildcard pattern case-insensitively
func matchesPattern(pattern, candidate string) bool {
	p := strings.ToLower(pattern)
	c := strings.ToLower(candidate)
	matched, err := filepath.Match(p, c)
	if err == nil && matched {
		return true
	}
	if strings.HasPrefix(p, "*") && strings.HasSuffix(p, "*") {
		sub := strings.Trim(p, "*")
		return strings.Contains(c, sub)
	}
	return false
}

// zaiCapabilities maps the z.ai catalog onto router capabilities. Every
// advertised z.ai model supports native tool use through the provider's prompt
// protocol, and all but the vision-only ones expose reasoning.
func zaiCapabilities(modelID string) (ModelCapabilities, bool) {
	table := map[string]ModelCapabilities{
		"x-preview-l": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true, Vision: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 200000, MaxOutput: 128000,
		},
		"glm-5.3": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 200000, MaxOutput: 128000,
		},
		"glm-5.2": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 200000, MaxOutput: 64064,
		},
		"GLM-5-Turbo": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 200000, MaxOutput: 32000,
		},
		"glm-4.7": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 200000, MaxOutput: 40000,
		},
		"GLM-5v-Turbo": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true, Vision: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 200000, MaxOutput: 32000,
		},
		"GLM-4.1V-Thinking-FlashX": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true, Vision: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 128000, MaxOutput: 16000,
		},
		"0727-106B-API": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 128000, MaxOutput: 80000,
		},
		"0727-360B-API": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 128000, MaxOutput: 80000,
		},
		"0808-360B-DR": {
			Tools: true, Reasoning: true, ThinkingCanDisable: true,
			ThinkingFormat: "openai", ThinkingLevels: []string{"low", "medium", "max"},
			ContextWindow: 128000, MaxOutput: 32000,
		},
	}
	caps, ok := table[modelID]
	return caps, ok
}

// ResolveCapabilities determines model capabilities based on provider and model ID.
func ResolveCapabilities(provider, modelID string) ModelCapabilities {
	baseID := strings.TrimSpace(modelID)
	// Advertised IDs are namespaced ("opencode/gpt-5.2"); patterns match the bare ID.
	baseID = strings.TrimPrefix(baseID, provider+"/")
	// Strip thinking suffix like "model(high)" if present
	if idx := strings.LastIndex(baseID, "("); idx != -1 && strings.HasSuffix(baseID, ")") {
		baseID = strings.TrimSpace(baseID[:idx])
	}

	if provider == "zai" {
		if caps, ok := zaiCapabilities(baseID); ok {
			return caps
		}
	}

	for _, rule := range patternRules {
		if matchesPattern(rule.pattern, baseID) {
			caps := rule.caps
			return caps
		}
	}

	// Default fallback (standard non-reasoning LLM)
	return ModelCapabilities{
		Tools:              true,
		Reasoning:          false,
		ThinkingCanDisable: true,
		ContextWindow:      200000,
		MaxOutput:          64000,
	}
}

// EnrichModelItem attaches capabilities, thinking levels, and token limits to ModelItem.
func EnrichModelItem(item *ModelItem, provider string) {
	if item == nil {
		return
	}
	caps := ResolveCapabilities(provider, item.ID)
	item.Capabilities = &caps
	if caps.Reasoning && len(caps.ThinkingLevels) > 0 {
		item.ThinkingLevels = caps.ThinkingLevels
	}
	if caps.ContextWindow > 0 {
		item.ContextLength = caps.ContextWindow
	}
	if caps.MaxOutput > 0 {
		item.MaxCompletionTokens = caps.MaxOutput
	}
}
