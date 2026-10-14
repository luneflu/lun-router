package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestAPIKeysWorkflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-router-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, err := NewKeyStore(tempDir)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	server := httptest.NewServer(setupRoutes(ks))
	defer server.Close()

	client := server.Client()

	// 1. Initially empty
	resp, err := client.Get(server.URL + "/api/keys")
	if err != nil {
		t.Fatalf("GET /api/keys: %v", err)
	}
	var listResp struct {
		Keys []APIKey `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(listResp.Keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(listResp.Keys))
	}

	// 2. Create auto-generated key
	autoPayload, _ := json.Marshal(CreateKeyRequest{Name: "Auto Key"})
	resp, err = client.Post(server.URL+"/api/keys", "application/json", bytes.NewReader(autoPayload))
	if err != nil {
		t.Fatalf("POST auto key: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}
	var autoKey APIKey
	if err := json.NewDecoder(resp.Body).Decode(&autoKey); err != nil {
		t.Fatalf("decode auto key: %v", err)
	}
	resp.Body.Close()

	if autoKey.Name != "Auto Key" {
		t.Errorf("expected name 'Auto Key', got '%s'", autoKey.Name)
	}
	if !strings.HasPrefix(autoKey.Key, "sk-") {
		t.Errorf("expected auto key prefix 'sk-', got '%s'", autoKey.Key)
	}
	parts := strings.Split(autoKey.Key, "-")
	if len(parts) != 4 {
		t.Errorf("expected 4 parts in auto key (sk-machineId-keyId-crc), got %d parts (%s)", len(parts), autoKey.Key)
	}

	// 3. Create custom key
	customPayload, _ := json.Marshal(CreateKeyRequest{
		Name: "Custom Key",
		Key:  "my-custom-secret-key-12345",
	})
	resp, err = client.Post(server.URL+"/api/keys", "application/json", bytes.NewReader(customPayload))
	if err != nil {
		t.Fatalf("POST custom key: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}
	var customKey APIKey
	if err := json.NewDecoder(resp.Body).Decode(&customKey); err != nil {
		t.Fatalf("decode custom key: %v", err)
	}
	resp.Body.Close()

	if customKey.Key != "my-custom-secret-key-12345" {
		t.Errorf("expected verbatim custom key, got '%s'", customKey.Key)
	}

	// 4. Verify custom key via verify endpoint
	req, _ := http.NewRequest("GET", server.URL+"/api/keys/verify", nil)
	req.Header.Set("Authorization", "Bearer my-custom-secret-key-12345")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("verify custom key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from verify, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Update key (toggle inactive)
	inactive := false
	newName := "Renamed Auto Key"
	updatePayload, _ := json.Marshal(UpdateKeyRequest{
		Name:     &newName,
		IsActive: &inactive,
	})
	putReq, _ := http.NewRequest("PUT", server.URL+"/api/keys/"+autoKey.ID, bytes.NewReader(updatePayload))
	putReq.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(putReq)
	if err != nil {
		t.Fatalf("PUT /api/keys: %v", err)
	}
	var updatedResp struct {
		Key APIKey `json:"key"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&updatedResp)
	resp.Body.Close()

	if updatedResp.Key.Name != "Renamed Auto Key" || updatedResp.Key.IsActive != false {
		t.Errorf("update failed: %+v", updatedResp.Key)
	}

	// 6. Verify inactive key is rejected
	req, _ = http.NewRequest("GET", server.URL+"/api/keys/verify", nil)
	req.Header.Set("Authorization", "Bearer "+autoKey.Key)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("verify inactive: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for inactive key, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 7. Delete key
	delReq, _ := http.NewRequest("DELETE", server.URL+"/api/keys/"+customKey.ID, nil)
	resp, err = client.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK on delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 8. Confirm count is 1
	resp, _ = client.Get(server.URL + "/api/keys")
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	if len(listResp.Keys) != 1 {
		t.Errorf("expected 1 remaining key, got %d", len(listResp.Keys))
	}
}

// Test OpenCode Free API: Header injection (Issue #4781), Fingerprint tools, and Completions
func TestOpenCodeCompletionsAndHeaders(t *testing.T) {
	sessionRegex := regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

	// 1. Mock upstream OpenCode server capturing received headers and body
	var receivedHeaders http.Header
	var receivedBody map[string]any

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		// Emit sample SSE chunks
		fmt.Fprintf(w, "data: {\"id\":\"cmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"big-pickle\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"id\":\"cmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"big-pickle\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world!\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockUpstream.Close()

	// 2. Setup KeyStore & Router
	tempDir, err := os.MkdirTemp("", "lun-opencode-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, err := NewKeyStore(tempDir)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	apiKey, err := ks.Create("Test Key", "sk-router-secret-key-1")
	if err != nil {
		t.Fatalf("Create key: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", handleOpenCodeChatCompletions(ks, mockUpstream.URL))
	mux.HandleFunc("GET /v1/models", handleOpenCodeModels)
	routerServer := httptest.NewServer(mux)
	defer routerServer.Close()

	client := routerServer.Client()

	// 3. Test Unauthorized request (missing key)
	unauthReq, _ := http.NewRequest("POST", routerServer.URL+"/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
	unauthReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(unauthReq)
	if err != nil {
		t.Fatalf("unauth req: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Test Non-streaming request with Issue #4781 verification
	compPayload := map[string]any{
		"model": "big-pickle",
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
		"stream": false,
	}
	payloadBytes, _ := json.Marshal(compPayload)
	req, _ := http.NewRequest("POST", routerServer.URL+"/v1/chat/completions", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey.Key)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("completion request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	var completionResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completionResp); err != nil {
		t.Fatalf("decode completion response: %v", err)
	}
	resp.Body.Close()

	// Verify aggregated text
	if completionResp.Choices[0].Message.Content != "Hello world!" {
		t.Errorf("expected aggregated 'Hello world!', got '%s'", completionResp.Choices[0].Message.Content)
	}

	// 5. Verify the 5 custom headers required by Issue #4781
	if receivedHeaders.Get("User-Agent") != "opencode/1.18.31" {
		t.Errorf("expected User-Agent 'opencode/1.18.31', got '%s'", receivedHeaders.Get("User-Agent"))
	}
	if receivedHeaders.Get("x-opencode-client") != "cli" {
		t.Errorf("expected x-opencode-client 'cli', got '%s'", receivedHeaders.Get("x-opencode-client"))
	}
	if receivedHeaders.Get("x-opencode-project") != "global" {
		t.Errorf("expected x-opencode-project 'global', got '%s'", receivedHeaders.Get("x-opencode-project"))
	}
	if receivedHeaders.Get("x-opencode-request") != "_msg" {
		t.Errorf("expected x-opencode-request '_msg', got '%s'", receivedHeaders.Get("x-opencode-request"))
	}
	sessionVal := receivedHeaders.Get("x-opencode-session")
	if !sessionRegex.MatchString(sessionVal) {
		t.Errorf("expected valid x-opencode-session (ses_<12hex><14base62>), got '%s'", sessionVal)
	}
	if receivedHeaders.Get("Authorization") != "Bearer public" {
		t.Errorf("expected Authorization 'Bearer public', got '%s'", receivedHeaders.Get("Authorization"))
	}

	// 6. Verify Fingerprint tools injected into body
	tools, ok := receivedBody["tools"].([]any)
	if !ok || len(tools) < 4 {
		t.Errorf("expected at least 4 fingerprint tools in body, got: %+v", receivedBody["tools"])
	} else {
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			if tMap, ok := tool.(map[string]any); ok {
				if fn, ok := tMap["function"].(map[string]any); ok {
					if name, ok := fn["name"].(string); ok {
						toolNames[name] = true
					}
				}
			}
		}
		for _, required := range []string{"bash", "glob", "grep", "read"} {
			if !toolNames[required] {
				t.Errorf("missing fingerprint tool: %s", required)
			}
		}
	}

	// 7. Test Streaming response
	compPayload["stream"] = true
	streamBytes, _ := json.Marshal(compPayload)
	streamReq, _ := http.NewRequest("POST", routerServer.URL+"/v1/chat/completions", bytes.NewReader(streamBytes))
	streamReq.Header.Set("Content-Type", "application/json")
	streamReq.Header.Set("Authorization", "Bearer "+apiKey.Key)

	streamResp, err := client.Do(streamReq)
	if err != nil {
		t.Fatalf("stream req: %v", err)
	}
	if streamResp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got '%s'", streamResp.Header.Get("Content-Type"))
	}
	streamBody, _ := io.ReadAll(streamResp.Body)
	streamResp.Body.Close()
	if !strings.Contains(string(streamBody), "Hello") {
		t.Errorf("expected stream body to contain 'Hello', got '%s'", string(streamBody))
	}
}

// Test live completion with real OpenCode Free upstream
func TestOpenCodeLiveCompletion(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping live test in CI")
	}

	tempDir, err := os.MkdirTemp("", "lun-live-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, _ := NewKeyStore(tempDir)
	key, _ := ks.Create("Live Test Key", "sk-live-key")

	server := httptest.NewServer(setupRoutes(ks))
	defer server.Close()

	payload := map[string]any{
		"model": "big-pickle",
		"messages": []map[string]string{
			{"role": "user", "content": "respond with the single word: PONG"},
		},
		"stream": false,
	}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", server.URL+"/v1/chat/completions", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key.Key)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("live request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK from live OpenCode, got %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}

	t.Logf("Live OpenCode Response: %q", res.Choices[0].Message.Content)
	if len(res.Choices[0].Message.Content) == 0 {
		t.Errorf("expected non-empty completion response")
	}
}

// Test /v1/models endpoint
func TestOpenCodeModelsEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", handleOpenCodeModels)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	resp.Body.Close()

	if len(data.Data) == 0 {
		t.Errorf("expected non-empty models list")
	}
}
