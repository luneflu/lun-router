package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lun-router/internal/chatgpt"
	"lun-router/internal/store"
)

func TestChatGPTDisabledByDefault(t *testing.T) {
	tempDir := t.TempDir()
	ms, _ := store.NewModelStore(tempDir)
	cgClient := chatgpt.NewClient(chatgpt.Config{DataDir: tempDir})

	compH := NewCompletionHandler(nil, ms, nil, nil, nil, nil, nil)
	compH.SetChatGPT(cgClient)

	// 1. Should be disabled by default
	if cgClient.IsEnabled() {
		t.Fatalf("expected ChatGPT provider to be disabled by default")
	}

	// 2. /v1/models should NOT contain chatgpt models
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	compH.Models(w, req)
	if strings.Contains(w.Body.String(), "chatgpt/chatgpt-4o") {
		t.Errorf("did not expect chatgpt/chatgpt-4o in /v1/models when provider is disabled")
	}

	// 3. Enabling without browser installed should fail
	reqPut := httptest.NewRequest(http.MethodPut, "/api/providers/chatgpt/status", strings.NewReader(`{"enabled":true}`))
	wPut := httptest.NewRecorder()
	compH.UpdateChatGPTStatus(wPut, reqPut)
	if wPut.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request when enabling without browser, got %d", wPut.Code)
	}

	// 4. Create fake dummy browser executable in dataDir/browser
	browserDir := filepath.Join(tempDir, "browser")
	_ = os.MkdirAll(browserDir, 0755)
	dummyBin := filepath.Join(browserDir, "chrome")
	_ = os.WriteFile(dummyBin, []byte("#!/bin/sh\nexit 0\n"), 0755)

	// Now enabling should succeed
	wPut2 := httptest.NewRecorder()
	reqPut2 := httptest.NewRequest(http.MethodPut, "/api/providers/chatgpt/status", strings.NewReader(`{"enabled":true}`))
	compH.UpdateChatGPTStatus(wPut2, reqPut2)
	if wPut2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after browser fake install, got %d: %s", wPut2.Code, wPut2.Body.String())
	}
	if !cgClient.IsEnabled() {
		t.Fatalf("expected client to be enabled")
	}

	// Now /v1/models should contain chatgpt models
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	compH.Models(w2, req2)
	if !strings.Contains(w2.Body.String(), "chatgpt/auto") {
		t.Errorf("expected chatgpt/auto in /v1/models when enabled")
	}
}

func TestChatGPTStatusEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	cgClient := chatgpt.NewClient(chatgpt.Config{DataDir: tempDir})
	compH := NewCompletionHandler(nil, nil, nil, nil, nil, nil, nil)
	compH.SetChatGPT(cgClient)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/chatgpt/status", nil)
	w := httptest.NewRecorder()
	compH.ChatGPTStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if res["enabled"] != false {
		t.Errorf("expected enabled=false")
	}
}

func TestChatGPTModelDisabled(t *testing.T) {
	tempDir := t.TempDir()
	ms, _ := store.NewModelStore(tempDir)
	_ = ms.SetDisabled("chatgpt", "auto", true)

	compH := NewCompletionHandler(nil, ms, nil, nil, nil, nil, nil)
	cgClient := chatgpt.NewClient(chatgpt.Config{DataDir: tempDir})
	compH.SetChatGPT(cgClient)

	res := compH.pingSingleModel(context.Background(), "chatgpt/auto", "hi", "")
	if res.OK {
		t.Errorf("expected disabled model test to fail")
	}
	if !strings.Contains(res.Error, "disabled") {
		t.Errorf("expected disabled error message, got: %s", res.Error)
	}
}

func TestChatGPTChatCompletionsNoClient(t *testing.T) {
	tempDir := t.TempDir()
	ks, _ := store.NewKeyStore(tempDir)
	key, _ := ks.Create("test-key", "sk-test-key-chatgpt")
	ms, _ := store.NewModelStore(tempDir)

	compH := NewCompletionHandler(ks, ms, nil, nil, nil, nil, nil)

	reqBody := `{"model":"chatgpt/chatgpt-4o","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+key.Key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	compH.ChatCompletions(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway, got %d", w.Code)
	}
}

func TestChatGPTChatCompletionsTimeout(t *testing.T) {
	tempDir := t.TempDir()
	ks, _ := store.NewKeyStore(tempDir)
	key, _ := ks.Create("test-key", "sk-test-key-chatgpt-2")
	ms, _ := store.NewModelStore(tempDir)

	cgClient := chatgpt.NewClient(chatgpt.Config{
		DataDir: tempDir,
		CDPURL:  "http://127.0.0.1:59999",
	})
	_ = cgClient.SetEnabled(true)

	compH := NewCompletionHandler(ks, ms, nil, nil, nil, nil, nil)
	compH.SetChatGPT(cgClient)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	reqBody := `{"model":"chatgpt/chatgpt-4o","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody)).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+key.Key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	compH.ChatCompletions(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
}
