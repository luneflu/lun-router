package model

import (
	"reflect"
	"testing"
)

func TestResolveCapabilitiesOpenCode(t *testing.T) {
	tests := []struct {
		modelID        string
		wantReasoning  bool
		wantFormat     string
		wantLevels     []string
		wantContext    int
		wantMaxOutput  int
	}{
		{
			modelID:       "muse-spark-1.3-contributor-free",
			wantReasoning: true,
			wantFormat:    "openai",
			wantLevels:    LevelsOpenAI,
			wantContext:   1048576,
			wantMaxOutput: 131072,
		},
		{
			modelID:       "muse-spark-1.2-contributor-free",
			wantReasoning: true,
			wantFormat:    "openai",
			wantLevels:    LevelsOpenAI,
			wantContext:   1048576,
			wantMaxOutput: 131072,
		},
		{
			modelID:       "deepseek-v4.1-flash-free",
			wantReasoning: true,
			wantFormat:    "deepseek",
			wantLevels:    LevelsDeepSeek4,
			wantContext:   1000000,
			wantMaxOutput: 128000,
		},
		{
			modelID:       "mimo-v2.6-flash-free",
			wantReasoning: true,
			wantFormat:    "deepseek",
			wantLevels:    LevelsMiMo,
			wantContext:   1048576,
			wantMaxOutput: 131072,
		},
		{
			modelID:       "gpt-5.2-codex",
			wantReasoning: true,
			wantFormat:    "openai",
			wantLevels:    LevelsCodex,
			wantContext:   400000,
			wantMaxOutput: 128000,
		},
		{
			modelID:       "big-pickle",
			wantReasoning: false,
			wantFormat:    "",
			wantLevels:    nil,
			wantContext:   200000,
			wantMaxOutput: 64000,
		},
		{
			modelID:       "space-bunny-free",
			wantReasoning: false,
			wantFormat:    "",
			wantLevels:    nil,
			wantContext:   200000,
			wantMaxOutput: 64000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			caps := ResolveCapabilities("opencode", tt.modelID)
			if caps.Reasoning != tt.wantReasoning {
				t.Errorf("%s: got reasoning %v, want %v", tt.modelID, caps.Reasoning, tt.wantReasoning)
			}
			if caps.ThinkingFormat != tt.wantFormat {
				t.Errorf("%s: got format %s, want %s", tt.modelID, caps.ThinkingFormat, tt.wantFormat)
			}
			if !reflect.DeepEqual(caps.ThinkingLevels, tt.wantLevels) {
				t.Errorf("%s: got levels %v, want %v", tt.modelID, caps.ThinkingLevels, tt.wantLevels)
			}
			if caps.ContextWindow != tt.wantContext {
				t.Errorf("%s: got context %d, want %d", tt.modelID, caps.ContextWindow, tt.wantContext)
			}
			if caps.MaxOutput != tt.wantMaxOutput {
				t.Errorf("%s: got max output %d, want %d", tt.modelID, caps.MaxOutput, tt.wantMaxOutput)
			}
		})
	}
}

func TestEnrichModelItem(t *testing.T) {
	item := &ModelItem{
		ID:      "muse-spark-1.3-contributor-free",
		Object:  "model",
		OwnedBy: "opencode",
	}

	EnrichModelItem(item, "opencode")

	if item.Capabilities == nil || !item.Capabilities.Reasoning {
		t.Fatalf("expected item.Capabilities.Reasoning to be true")
	}
	if len(item.ThinkingLevels) != len(LevelsOpenAI) {
		t.Fatalf("expected %d thinking levels, got %d", len(LevelsOpenAI), len(item.ThinkingLevels))
	}
	if item.ContextLength != 1048576 {
		t.Errorf("expected context_length 1048576, got %d", item.ContextLength)
	}
	if item.MaxCompletionTokens != 131072 {
		t.Errorf("expected max_completion_tokens 131072, got %d", item.MaxCompletionTokens)
	}
}
