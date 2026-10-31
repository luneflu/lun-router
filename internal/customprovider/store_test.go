package customprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStoreCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "customprov-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// 1. Add valid provider
	p, err := store.Add(Provider{
		Name:      "Groq",
		Prefix:    "groq",
		BaseURL:   "https://api.groq.com/openai/v1/",
		APIKey:    "gsk-1234567890abcdef",
		IsEnabled: true,
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if p.ID == "" {
		t.Errorf("expected generated ID")
	}
	if p.Prefix != "groq" {
		t.Errorf("expected prefix 'groq', got %q", p.Prefix)
	}
	if p.BaseURL != "https://api.groq.com/openai/v1" {
		t.Errorf("expected trailing slash stripped, got %q", p.BaseURL)
	}

	// 2. Reject duplicate prefix
	_, err = store.Add(Provider{
		Name:    "Groq 2",
		Prefix:  "groq",
		BaseURL: "https://api.groq.com/openai/v1",
	})
	if err == nil {
		t.Errorf("expected error for duplicate prefix")
	}

	// 3. Reject reserved prefix
	_, err = store.Add(Provider{
		Name:    "OpenCode Free Copy",
		Prefix:  "opencode",
		BaseURL: "https://example.com/v1",
	})
	if err == nil {
		t.Errorf("expected error for reserved prefix 'opencode'")
	}

	// 4. Reject invalid prefix format
	_, err = store.Add(Provider{
		Name:    "Invalid",
		Prefix:  "my prefix!",
		BaseURL: "https://example.com/v1",
	})
	if err == nil {
		t.Errorf("expected error for invalid prefix format")
	}

	// 5. Get and GetByPrefix
	got, ok := store.Get(p.ID)
	if !ok || got.Name != "Groq" {
		t.Errorf("Get failed or wrong provider")
	}

	gotByPrefix, ok := store.GetByPrefix("groq")
	if !ok || gotByPrefix.ID != p.ID {
		t.Errorf("GetByPrefix failed")
	}

	// 6. Update
	updated, err := store.Update(p.ID, func(target *Provider) error {
		target.Name = "Groq Fast"
		target.Prefix = "groq-fast"
		return nil
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "Groq Fast" || updated.Prefix != "groq-fast" {
		t.Errorf("Update fields mismatch")
	}

	// 7. SetModels
	err = store.SetModels(p.ID, []Model{
		{ID: "llama-3.3-70b", Name: "Llama 3.3 70B"},
		{ID: "mixtral-8x7b", Name: "Mixtral 8x7B"},
		{ID: "llama-3.3-70b", Name: "Llama 3.3 70B Duplicate"},
	})
	if err != nil {
		t.Fatalf("SetModels failed: %v", err)
	}
	gotModels, _ := store.Get(p.ID)
	if len(gotModels.Models) != 2 {
		t.Errorf("expected 2 unique models, got %d", len(gotModels.Models))
	}

	// 8. FindByModel
	foundByModel, ok := store.FindByModel("llama-3.3-70b")
	if !ok || foundByModel.ID != p.ID {
		t.Errorf("FindByModel failed")
	}

	// 9. Persistence
	store2, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("re-open store failed: %v", err)
	}
	if len(store2.List()) != 1 {
		t.Fatalf("expected 1 persisted provider, got %d", len(store2.List()))
	}

	// 10. Delete
	err = store2.Delete(p.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(store2.List()) != 0 {
		t.Errorf("expected 0 providers after delete")
	}
}

func TestClientFetchModelsAndExecuteChat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "llama-3.3-70b"},
					{"id": "deepseek-r1"},
				},
			})
		case "/v1/chat/completions":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["model"] != "llama-3.3-70b" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("model prefix was not stripped"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-123",
				"choices": []map[string]any{{"message": map[string]string{"content": "hello"}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewClient()
	p := Provider{
		ID:        "cp_test",
		Name:      "Mock OpenAI",
		Prefix:    "mock",
		BaseURL:   ts.URL + "/v1",
		APIKey:    "test-key",
		IsEnabled: true,
	}

	// FetchModels
	models, err := client.FetchModels(context.Background(), p)
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}
	if len(models) != 2 || models[0].ID != "deepseek-r1" || models[1].ID != "llama-3.3-70b" {
		t.Errorf("unexpected models: %+v", models)
	}

	// ExecuteChat
	resp, err := client.ExecuteChat(context.Background(), p, map[string]any{
		"model": "mock/llama-3.3-70b",
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
	}, false)
	if err != nil {
		t.Fatalf("ExecuteChat failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ExecuteChat status code %d", resp.StatusCode)
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestParseModelFromMap_PrecedenceAndFields(t *testing.T) {
	// 1. Name precedence: name, display_name, id
	t.Run("NamePrecedence", func(t *testing.T) {
		m1, _ := ParseModelFromMap(map[string]any{
			"id":           "m-1",
			"name":         "Custom Name",
			"display_name": "Display Name",
		}, nil)
		if m1.Name != "Custom Name" {
			t.Errorf("got %q, want Custom Name", m1.Name)
		}

		m2, _ := ParseModelFromMap(map[string]any{
			"id":           "m-2",
			"display_name": "Display Name",
		}, nil)
		if m2.Name != "Display Name" {
			t.Errorf("got %q, want Display Name", m2.Name)
		}

		m3, _ := ParseModelFromMap(map[string]any{
			"id": "m-3",
		}, nil)
		if m3.Name != "m-3" {
			t.Errorf("got %q, want m-3", m3.Name)
		}
	})

	// 2. ContextWindow precedence & validation
	t.Run("ContextWindowPrecedence", func(t *testing.T) {
		// context_window wins over contextWindow, max_model_len, etc.
		m, _ := ParseModelFromMap(map[string]any{
			"id":                 "m",
			"context_window":     100000,
			"contextWindow":      90000,
			"max_model_len":      80000,
			"context_length":     70000,
			"max_context_length": 60000,
			"max_input_tokens":   50000,
		}, nil)
		if m.ContextWindow != 100000 {
			t.Errorf("got %d, want 100000", m.ContextWindow)
		}

		// Fallback order: contextWindow -> max_model_len -> context_length -> max_context_length -> max_input_tokens
		mCW, _ := ParseModelFromMap(map[string]any{
			"id":            "m",
			"contextWindow": 90000,
			"max_model_len": 80000,
		}, nil)
		if mCW.ContextWindow != 90000 {
			t.Errorf("got %d, want 90000", mCW.ContextWindow)
		}

		mVLLM, _ := ParseModelFromMap(map[string]any{
			"id":            "m",
			"max_model_len": 80000,
		}, nil)
		if mVLLM.ContextWindow != 80000 {
			t.Errorf("got %d, want 80000", mVLLM.ContextWindow)
		}

		mContextLength, _ := ParseModelFromMap(map[string]any{
			"id":             "m",
			"context_length": 70000,
		}, nil)
		if mContextLength.ContextWindow != 70000 {
			t.Errorf("got %d, want 70000", mContextLength.ContextWindow)
		}

		mAnthropic, _ := ParseModelFromMap(map[string]any{
			"id":               "m",
			"max_input_tokens": 200000,
		}, nil)
		if mAnthropic.ContextWindow != 200000 {
			t.Errorf("got %d, want 200000", mAnthropic.ContextWindow)
		}

		// Non-positive limits fall through
		mInvalid, _ := ParseModelFromMap(map[string]any{
			"id":             "m",
			"context_window": 0,
			"contextWindow":  -5,
			"max_model_len":  32000,
		}, nil)
		if mInvalid.ContextWindow != 32000 {
			t.Errorf("got %d, want 32000", mInvalid.ContextWindow)
		}
	})

	// 3. MaxTokens precedence & validation
	t.Run("MaxTokensPrecedence", func(t *testing.T) {
		m, _ := ParseModelFromMap(map[string]any{
			"id":                    "m",
			"max_tokens":            8192,
			"maxTokens":             4096,
			"max_output_tokens":     2048,
			"max_completion_tokens": 1024,
		}, nil)
		if m.MaxTokens != 8192 {
			t.Errorf("got %d, want 8192", m.MaxTokens)
		}

		mFall1, _ := ParseModelFromMap(map[string]any{
			"id":                "m",
			"maxTokens":         4096,
			"max_output_tokens": 2048,
		}, nil)
		if mFall1.MaxTokens != 4096 {
			t.Errorf("got %d, want 4096", mFall1.MaxTokens)
		}

		mFall2, _ := ParseModelFromMap(map[string]any{
			"id":                "m",
			"max_output_tokens": 2048,
		}, nil)
		if mFall2.MaxTokens != 2048 {
			t.Errorf("got %d, want 2048", mFall2.MaxTokens)
		}

		mFall3, _ := ParseModelFromMap(map[string]any{
			"id":                    "m",
			"max_completion_tokens": 1024,
		}, nil)
		if mFall3.MaxTokens != 1024 {
			t.Errorf("got %d, want 1024", mFall3.MaxTokens)
		}

		// Non-positive values fall through
		mInvalid, _ := ParseModelFromMap(map[string]any{
			"id":                    "m",
			"max_tokens":            0,
			"max_completion_tokens": 1024,
		}, nil)
		if mInvalid.MaxTokens != 1024 {
			t.Errorf("got %d, want 1024", mInvalid.MaxTokens)
		}
	})

	// 4. Input precedence & capabilities fallback
	t.Run("InputPrecedence", func(t *testing.T) {
		// Explicit input with text and image
		m1, _ := ParseModelFromMap(map[string]any{
			"id":    "m",
			"input": []any{"text", "image"},
		}, nil)
		if len(m1.Input) != 2 || m1.Input[0] != "text" || m1.Input[1] != "image" {
			t.Errorf("got %v, want [text image]", m1.Input)
		}

		// Explicit input with only text
		m2, _ := ParseModelFromMap(map[string]any{
			"id":    "m",
			"input": []any{"text"},
			"capabilities": map[string]any{
				"vision": true, // explicit input overrides capabilities
			},
		}, nil)
		if len(m2.Input) != 1 || m2.Input[0] != "text" {
			t.Errorf("got %v, want [text]", m2.Input)
		}

		// Explicit input with only image -> always includes text
		m3, _ := ParseModelFromMap(map[string]any{
			"id":    "m",
			"input": []any{"image"},
		}, nil)
		if len(m3.Input) != 2 || m3.Input[0] != "text" || m3.Input[1] != "image" {
			t.Errorf("got %v, want [text image]", m3.Input)
		}

		// Fallback to capabilities.vision boolean
		m4, _ := ParseModelFromMap(map[string]any{
			"id": "m",
			"capabilities": map[string]any{
				"vision": true,
			},
		}, nil)
		if len(m4.Input) != 2 || m4.Input[0] != "text" || m4.Input[1] != "image" {
			t.Errorf("got %v, want [text image]", m4.Input)
		}

		// Fallback to capabilities.vision object
		m5, _ := ParseModelFromMap(map[string]any{
			"id": "m",
			"capabilities": map[string]any{
				"vision": map[string]any{"supported": true},
			},
		}, nil)
		if len(m5.Input) != 2 || m5.Input[0] != "text" || m5.Input[1] != "image" {
			t.Errorf("got %v, want [text image]", m5.Input)
		}

		// Fallback to capabilities.image_input
		m6, _ := ParseModelFromMap(map[string]any{
			"id": "m",
			"capabilities": map[string]any{
				"image_input": true,
			},
		}, nil)
		if len(m6.Input) != 2 || m6.Input[0] != "text" || m6.Input[1] != "image" {
			t.Errorf("got %v, want [text image]", m6.Input)
		}

		// Explicit false preserved: no image support
		m7, _ := ParseModelFromMap(map[string]any{
			"id": "m",
			"capabilities": map[string]any{
				"vision": map[string]any{"supported": false},
			},
		}, nil)
		if len(m7.Input) != 1 || m7.Input[0] != "text" {
			t.Errorf("got %v, want [text]", m7.Input)
		}

		// Neither specified -> defaults to text only
		m8, _ := ParseModelFromMap(map[string]any{
			"id": "m",
		}, nil)
		if len(m8.Input) != 1 || m8.Input[0] != "text" {
			t.Errorf("got %v, want [text]", m8.Input)
		}
	})

	// 5. Reasoning precedence
	t.Run("ReasoningPrecedence", func(t *testing.T) {
		// Provider compat.reasoning overrides everything
		compatFalse := &ProviderCompat{Reasoning: boolPtr(false)}
		m1, _ := ParseModelFromMap(map[string]any{
			"id":        "m",
			"reasoning": true,
		}, compatFalse)
		if m1.Reasoning == nil || *m1.Reasoning != false {
			t.Errorf("expected compat.reasoning false override")
		}

		compatTrue := &ProviderCompat{Reasoning: boolPtr(true)}
		m2, _ := ParseModelFromMap(map[string]any{
			"id":        "m",
			"reasoning": false,
		}, compatTrue)
		if m2.Reasoning == nil || *m2.Reasoning != true {
			t.Errorf("expected compat.reasoning true override")
		}

		// Model reasoning
		m3, _ := ParseModelFromMap(map[string]any{
			"id":        "m",
			"reasoning": true,
			"capabilities": map[string]any{
				"reasoning": false,
			},
		}, nil)
		if m3.Reasoning == nil || *m3.Reasoning != true {
			t.Errorf("expected model reasoning true")
		}

		// Model reasoning as object
		m4, _ := ParseModelFromMap(map[string]any{
			"id":        "m",
			"reasoning": map[string]any{"supported": false},
		}, nil)
		if m4.Reasoning == nil || *m4.Reasoning != false {
			t.Errorf("expected model reasoning false preserved")
		}

		// capabilities.reasoning
		m5, _ := ParseModelFromMap(map[string]any{
			"id": "m",
			"capabilities": map[string]any{
				"reasoning": true,
				"thinking":  false,
			},
		}, nil)
		if m5.Reasoning == nil || *m5.Reasoning != true {
			t.Errorf("expected capabilities.reasoning true")
		}

		// capabilities.thinking
		m6, _ := ParseModelFromMap(map[string]any{
			"id": "m",
			"capabilities": map[string]any{
				"thinking": map[string]any{"supported": true},
			},
		}, nil)
		if m6.Reasoning == nil || *m6.Reasoning != true {
			t.Errorf("expected capabilities.thinking true")
		}

		// Undefined reasoning -> nil
		m7, _ := ParseModelFromMap(map[string]any{
			"id": "m",
		}, nil)
		if m7.Reasoning != nil {
			t.Errorf("expected nil reasoning, got %v", *m7.Reasoning)
		}
	})
}

func TestModelUnmarshalJSON_Compatibility(t *testing.T) {
	// String array (legacy format)
	legacyJSON := `["llama-3.3-70b", "deepseek-r1"]`
	var legacyModels []Model
	if err := json.Unmarshal([]byte(legacyJSON), &legacyModels); err != nil {
		t.Fatalf("unmarshal legacy string array: %v", err)
	}
	if len(legacyModels) != 2 || legacyModels[0].ID != "llama-3.3-70b" || legacyModels[1].ID != "deepseek-r1" {
		t.Errorf("unexpected legacy models: %+v", legacyModels)
	}
	if len(legacyModels[0].Input) != 1 || legacyModels[0].Input[0] != "text" {
		t.Errorf("expected legacy models to have input [text]")
	}

	// Full object array (new format)
	newJSON := `[
		{
			"id": "deepseek-r1",
			"name": "DeepSeek R1",
			"contextWindow": 65536,
			"maxTokens": 8192,
			"input": ["text"],
			"reasoning": true
		}
	]`
	var newModels []Model
	if err := json.Unmarshal([]byte(newJSON), &newModels); err != nil {
		t.Fatalf("unmarshal new model objects: %v", err)
	}
	if len(newModels) != 1 || newModels[0].Name != "DeepSeek R1" || newModels[0].ContextWindow != 65536 ||
		newModels[0].MaxTokens != 8192 || newModels[0].Reasoning == nil || !*newModels[0].Reasoning {
		t.Errorf("unexpected unmarshaled model: %+v", newModels[0])
	}
}
