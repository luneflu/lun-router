package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"lun-router/internal/antigravity"
	"lun-router/internal/caveman"
	"lun-router/internal/handler"
	"lun-router/internal/model"
	"lun-router/internal/opencode"
	"lun-router/internal/rtk"
	"lun-router/internal/store"
)

func TestAPIKeysWorkflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-router-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, err := store.NewKeyStore(tempDir)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	ms, err := store.NewModelStore(tempDir)
	if err != nil {
		t.Fatalf("NewModelStore: %v", err)
	}

	server := httptest.NewServer(handler.NewRouter(ks, ms, nil, nil, nil, nil))
	defer server.Close()

	client := server.Client()

	// 1. Initially empty
	resp, err := client.Get(server.URL + "/api/keys")
	if err != nil {
		t.Fatalf("GET /api/keys: %v", err)
	}
	var listResp struct {
		Keys []model.APIKey `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(listResp.Keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(listResp.Keys))
	}

	// 2. Create auto-generated key
	autoPayload, _ := json.Marshal(model.CreateKeyRequest{Name: "Auto Key"})
	resp, err = client.Post(server.URL+"/api/keys", "application/json", bytes.NewReader(autoPayload))
	if err != nil {
		t.Fatalf("POST auto key: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}
	var autoKey model.APIKey
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
	customPayload, _ := json.Marshal(model.CreateKeyRequest{
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
	var customKey model.APIKey
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
	updatePayload, _ := json.Marshal(model.UpdateKeyRequest{
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
		Key model.APIKey `json:"key"`
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

	var receivedHeaders http.Header
	var receivedBody map[string]any

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"cmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"big-pickle\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"id\":\"cmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"big-pickle\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world!\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockUpstream.Close()

	tempDir, err := os.MkdirTemp("", "lun-opencode-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, err := store.NewKeyStore(tempDir)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}

	apiKey, err := ks.Create("Test Key", "sk-router-secret-key-1")
	if err != nil {
		t.Fatalf("Create key: %v", err)
	}

	ms, _ := store.NewModelStore(tempDir)
	mockClient := opencode.NewClient(mockUpstream.URL, nil)
	routerServer := httptest.NewServer(handler.NewRouter(ks, ms, mockClient, nil, nil, nil))
	defer routerServer.Close()

	client := routerServer.Client()

	// 1. Test Unauthorized request (missing key)
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

	// 2. Test Non-streaming request with Issue #4781 verification
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

	if completionResp.Choices[0].Message.Content != "Hello world!" {
		t.Errorf("expected aggregated 'Hello world!', got '%s'", completionResp.Choices[0].Message.Content)
	}

	// 3. Verify the 5 custom headers required by Issue #4781
	expectedUA := mockClient.VersionManager().GetUserAgent()
	if receivedHeaders.Get("User-Agent") != expectedUA {
		t.Errorf("expected User-Agent '%s', got '%s'", expectedUA, receivedHeaders.Get("User-Agent"))
	}
	if receivedHeaders.Get("x-opencode-client") != opencode.ClientType {
		t.Errorf("expected x-opencode-client '%s', got '%s'", opencode.ClientType, receivedHeaders.Get("x-opencode-client"))
	}
	if receivedHeaders.Get("x-opencode-project") != opencode.Project {
		t.Errorf("expected x-opencode-project '%s', got '%s'", opencode.Project, receivedHeaders.Get("x-opencode-project"))
	}
	sessionVal := receivedHeaders.Get("x-opencode-session")
	if !sessionRegex.MatchString(sessionVal) {
		t.Errorf("expected valid x-opencode-session (ses_<12hex><14base62>), got '%s'", sessionVal)
	}
	if receivedHeaders.Get("x-opencode-session-id") != sessionVal {
		t.Errorf("expected x-opencode-session-id to match session, got '%s'", receivedHeaders.Get("x-opencode-session-id"))
	}
	if receivedHeaders.Get("x-session-affinity") != sessionVal {
		t.Errorf("expected x-session-affinity to match session, got '%s'", receivedHeaders.Get("x-session-affinity"))
	}
	if receivedHeaders.Get("Authorization") != "Bearer public" {
		t.Errorf("expected Authorization 'Bearer public', got '%s'", receivedHeaders.Get("Authorization"))
	}

	// 4. Verify Fingerprint tools injected into body (v2 required: read, shell)
	tools, ok := receivedBody["tools"].([]any)
	if !ok || len(tools) < 2 {
		t.Errorf("expected at least 2 fingerprint tools in body, got: %+v", receivedBody["tools"])
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
		for _, required := range []string{"read", "shell"} {
			if !toolNames[required] {
				t.Errorf("missing fingerprint tool: %s", required)
			}
		}
	}

	// 5. Test Streaming response
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

	ks, _ := store.NewKeyStore(tempDir)
	key, _ := ks.Create("Live Test Key", "sk-live-key")

	ms, _ := store.NewModelStore(tempDir)
	server := httptest.NewServer(handler.NewRouter(ks, ms, nil, nil, nil, nil))
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
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			t.Skipf("Live OpenCode returned %d (rate-limit / temporary upstream gate): %s", resp.StatusCode, string(b))
			return
		}
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
	compH := handler.NewCompletionHandler(nil, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", compH.Models)
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

// Test Provider Models enable/disable toggle & completion rejection
func TestOpenCodeModelEnableDisable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-models-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	key, _ := ks.Create("Admin Key", "sk-test-admin-key")

	server := httptest.NewServer(handler.NewRouter(ks, ms, nil, nil, nil, nil))
	defer server.Close()

	client := server.Client()

	// 1. Initial list from /api/providers/opencode/models
	resp, err := client.Get(server.URL + "/api/providers/opencode/models")
	if err != nil {
		t.Fatalf("GET provider models: %v", err)
	}
	var provResp struct {
		Provider string              `json:"provider"`
		Models   []handler.ModelItem `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&provResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	if len(provResp.Models) == 0 {
		t.Fatalf("expected non-empty models list")
	}

	targetModel := "big-pickle"
	var found bool
	for _, m := range provResp.Models {
		if m.ID == targetModel {
			found = true
			if !m.Enabled {
				t.Errorf("expected model %s to be enabled initially", targetModel)
			}
			break
		}
	}
	if !found {
		t.Fatalf("model %s not found in provider models", targetModel)
	}

	// 2. Disable targetModel
	disablePayload, _ := json.Marshal(map[string]any{
		"model":   targetModel,
		"enabled": false,
	})
	putReq, _ := http.NewRequest("PUT", server.URL+"/api/providers/opencode/models", bytes.NewReader(disablePayload))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := client.Do(putReq)
	if err != nil {
		t.Fatalf("PUT disable model: %v", err)
	}
	if putResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", putResp.StatusCode)
	}
	putResp.Body.Close()

	// 3. Confirm targetModel is disabled in /api/providers/opencode/models
	var provResp2 struct {
		Provider string              `json:"provider"`
		Models   []handler.ModelItem `json:"models"`
	}
	resp, _ = client.Get(server.URL + "/api/providers/opencode/models")
	_ = json.NewDecoder(resp.Body).Decode(&provResp2)
	resp.Body.Close()
	var targetModelDisabled bool
	for _, m := range provResp2.Models {
		if m.ID == targetModel {
			if m.Enabled {
				t.Errorf("expected model %s to be disabled now", targetModel)
			} else {
				targetModelDisabled = true
			}
		}
	}
	if !targetModelDisabled {
		t.Errorf("model %s was not found or not marked disabled in provResp2", targetModel)
	}

	// 4. Confirm targetModel is excluded from /v1/models
	v1Resp, _ := client.Get(server.URL + "/v1/models")
	var v1Data struct {
		Data []handler.ModelItem `json:"data"`
	}
	_ = json.NewDecoder(v1Resp.Body).Decode(&v1Data)
	v1Resp.Body.Close()
	for _, m := range v1Data.Data {
		if m.ID == targetModel {
			t.Errorf("model %s should NOT appear in /v1/models when disabled", targetModel)
		}
	}

	// 5. Confirm /v1/chat/completions rejects disabled model with 400
	chatPayload, _ := json.Marshal(map[string]any{
		"model":    targetModel,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	chatReq, _ := http.NewRequest("POST", server.URL+"/v1/chat/completions", bytes.NewReader(chatPayload))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+key.Key)
	chatResp, err := client.Do(chatReq)
	if err != nil {
		t.Fatalf("chat request: %v", err)
	}
	if chatResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for disabled model, got %d", chatResp.StatusCode)
	}
	chatResp.Body.Close()

	// 6. Re-enable targetModel
	enablePayload, _ := json.Marshal(map[string]any{
		"model":   targetModel,
		"enabled": true,
	})
	putReq2, _ := http.NewRequest("PUT", server.URL+"/api/providers/opencode/models", bytes.NewReader(enablePayload))
	putReq2.Header.Set("Content-Type", "application/json")
	putResp2, _ := client.Do(putReq2)
	putResp2.Body.Close()

	// 7. Confirm targetModel is back in /v1/models
	v1Resp2, _ := client.Get(server.URL + "/v1/models")
	_ = json.NewDecoder(v1Resp2.Body).Decode(&v1Data)
	v1Resp2.Body.Close()
	var reEnabledFound bool
	for _, m := range v1Data.Data {
		if m.ID == targetModel {
			reEnabledFound = true
			break
		}
	}
	if !reEnabledFound {
		t.Errorf("expected %s to reappear in /v1/models after re-enabling", targetModel)
	}
}

// Test OpenCode session rotation when upstream rate limits with HTTP 429
func TestOpenCodeSessionRotationOnRateLimit(t *testing.T) {
	var requestCount int
	var sessionsSeen []string

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		currentSession := r.Header.Get("x-opencode-session")
		sessionsSeen = append(sessionsSeen, currentSession)

		if requestCount == 1 {
			// Simulate upstream 429 FreeUsageLimitError
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"FreeUsageLimitError","message":"Session limit reached"}}`))
			return
		}

		// Subsequent request with rotated session succeeds
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"cmpl-rotated\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"big-pickle\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Recovered from 429!\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockUpstream.Close()

	tempDir, err := os.MkdirTemp("", "lun-ratelimit-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	key, _ := ks.Create("Rate Limit Test Key", "sk-ratelimit-key-1")

	client := opencode.NewClient(mockUpstream.URL, nil)
	router := handler.NewRouter(ks, ms, client, nil, nil, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	// Initial session for this key
	initialSession := client.SessionManager().Get(key.Key)
	if initialSession == "" {
		t.Fatalf("expected non-empty initial session")
	}

	// Send request: it should hit 429 once, rotate session, and retry transparently
	payload := map[string]any{
		"model": "big-pickle",
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
		"stream": false,
	}
	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", server.URL+"/v1/chat/completions", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key.Key)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK after session rotation, got %d: %s", resp.StatusCode, string(body))
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

	if res.Choices[0].Message.Content != "Recovered from 429!" {
		t.Errorf("unexpected content: %s", res.Choices[0].Message.Content)
	}

	// Verify that 2 requests were made and the second had a DIFFERENT, brand new session ID
	if requestCount != 2 {
		t.Errorf("expected 2 upstream requests (initial + retry), got %d", requestCount)
	}
	if len(sessionsSeen) != 2 {
		t.Fatalf("expected 2 sessions recorded, got %d", len(sessionsSeen))
	}
	if sessionsSeen[0] == sessionsSeen[1] {
		t.Errorf("expected rotated session ID on retry, but got same: %s", sessionsSeen[0])
	}

	// Verify the session manager now holds the new rotated session
	currentStored := client.SessionManager().Get(key.Key)
	if currentStored != sessionsSeen[1] {
		t.Errorf("expected session manager to store new rotated session %s, got %s", sessionsSeen[1], currentStored)
	}
}

// Test /api/models/test: Ping model with empty prompt
func TestOpenCodeModelTestPing(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Verify probe prompt was sent
		messages, _ := body["messages"].([]any)
		if len(messages) > 0 {
			if first, ok := messages[0].(map[string]any); ok {
				if first["content"] != "ping" && first["content"] != "" {
					t.Errorf("expected probe prompt content 'ping' or '', got: %v", first["content"])
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"cmpl-ping\",\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer mockUpstream.Close()

	tempDir, _ := os.MkdirTemp("", "lun-ping-test-*")
	defer os.RemoveAll(tempDir)

	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	client := opencode.NewClient(mockUpstream.URL, nil)

	router := handler.NewRouter(ks, ms, client, nil, nil, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	// 1. Ping enabled model
	pingPayload, _ := json.Marshal(map[string]string{
		"model": "big-pickle",
	})
	resp, err := server.Client().Post(server.URL+"/api/models/test", "application/json", bytes.NewReader(pingPayload))
	if err != nil {
		t.Fatalf("POST /api/models/test: %v", err)
	}
	var result struct {
		OK        bool   `json:"ok"`
		Model     string `json:"model"`
		LatencyMs int64  `json:"latency_ms"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if !result.OK || result.Model != "big-pickle" {
		t.Errorf("expected ok: true for big-pickle, got %+v", result)
	}

	// 2. Disable model and verify ping returns ok: false
	_ = ms.SetDisabled("opencode", "big-pickle", true)

	resp, _ = server.Client().Post(server.URL+"/api/models/test", "application/json", bytes.NewReader(pingPayload))
	var disabledResult struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&disabledResult)
	resp.Body.Close()

	if disabledResult.OK {
		t.Errorf("expected ok: false for disabled model, got true")
	}
}

// Test OpenCode Version Manager & Cache
func TestOpenCodeVersionManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-version-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	vm := opencode.NewVersionManager(tempDir, 1*time.Hour)
	if vm.GetVersion() != opencode.DefaultVersion {
		t.Errorf("expected default version %s, got %s", opencode.DefaultVersion, vm.GetVersion())
	}
	expectedUA := fmt.Sprintf("opencode/latest/%s/cli", opencode.DefaultVersion)
	if vm.GetUserAgent() != expectedUA {
		t.Errorf("expected UA %s, got %s", expectedUA, vm.GetUserAgent())
	}

	// Test updating version
	vm.SetVersion("2.0.29")
	if vm.GetVersion() != "2.0.29" {
		t.Errorf("expected updated version 2.0.29, got %s", vm.GetVersion())
	}
	expectedUA2 := "opencode/latest/2.0.29/cli"
	if vm.GetUserAgent() != expectedUA2 {
		t.Errorf("expected UA %s, got %s", expectedUA2, vm.GetUserAgent())
	}

	// Verify persistence: second manager loads cached version from file
	vm2 := opencode.NewVersionManager(tempDir, 1*time.Hour)
	if vm2.GetVersion() != "2.0.29" {
		t.Errorf("expected loaded cached version 2.0.29, got %s", vm2.GetVersion())
	}

	// Test /api/providers/opencode/models returns the version
	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	client := opencode.NewClient("", vm2)
	router := handler.NewRouter(ks, ms, client, nil, nil, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/api/providers/opencode/models")
	if err != nil {
		t.Fatalf("GET /api/providers/opencode/models: %v", err)
	}
	var provData struct {
		Provider string `json:"provider"`
		Version  string `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&provData)
	resp.Body.Close()

	if provData.Version != "2.0.29" {
		t.Errorf("expected provider models version 2.0.29, got %s", provData.Version)
	}
}

func TestSPAStaticServing(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!DOCTYPE html><html><body><h1>Lun Router</h1></body></html>"),
		},
		"robots.txt": &fstest.MapFile{
			Data: []byte("User-agent: *\nDisallow:"),
		},
	}

	router := handler.NewRouter(nil, nil, nil, nil, nil, nil, mockFS)
	server := httptest.NewServer(router)
	defer server.Close()

	client := server.Client()

	// 1. Root /
	resp, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Lun Router") {
		t.Errorf("expected HTML body, got %s", string(body))
	}

	// 2. Static asset /robots.txt
	resp, err = client.Get(server.URL + "/robots.txt")
	if err != nil {
		t.Fatalf("GET /robots.txt: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /robots.txt, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "User-agent") {
		t.Errorf("expected robots.txt body, got %s", string(body))
	}

	// 3. SPA Route fallback /opencode -> index.html
	resp, err = client.Get(server.URL + "/opencode")
	if err != nil {
		t.Fatalf("GET /opencode: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for /opencode fallback, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Lun Router") {
		t.Errorf("expected fallback to index.html, got %s", string(body))
	}

	// 4. API 404 does NOT fall back to index.html
	resp, err = client.Get(server.URL + "/api/unknown-endpoint")
	if err != nil {
		t.Fatalf("GET /api/unknown-endpoint: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown API path, got %d", resp.StatusCode)
	}

	// 5. POST on static route returns 405
	resp, err = client.Post(server.URL+"/some-page", "text/plain", nil)
	if err != nil {
		t.Fatalf("POST /some-page: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST to static route, got %d", resp.StatusCode)
	}
}

func TestResponsesAssistantOutputText(t *testing.T) {
	var receivedBody map[string]any

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inference/v1/responses" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello back\"}\n\n")
			fmt.Fprintf(w, "data: [DONE]\n\n")
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	client := opencode.NewClient(upstream.URL, nil)

	ctx := context.Background()
	bodyMap := map[string]any{
		"model": "muse-spark-1.2-contributor-free",
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
			{"role": "user", "content": "How are you?"},
		},
	}

	resp, err := client.ExecuteChat(ctx, bodyMap, "test", "")
	if err != nil {
		t.Fatalf("ExecuteChat: %v", err)
	}
	resp.Body.Close()

	inputList, ok := receivedBody["input"].([]any)
	if !ok || len(inputList) != 3 {
		t.Fatalf("expected 3 input items in Responses API body, got %v", receivedBody["input"])
	}

	// Verify user message has type: input_text
	userMsg1 := inputList[0].(map[string]any)
	userContent1 := userMsg1["content"].([]any)[0].(map[string]any)
	if userContent1["type"] != "input_text" {
		t.Errorf("expected input_text for user, got %v", userContent1["type"])
	}

	// Verify assistant message has type: output_text (NOT input_text!)
	asstMsg := inputList[1].(map[string]any)
	asstContent := asstMsg["content"].([]any)[0].(map[string]any)
	if asstContent["type"] != "output_text" {
		t.Errorf("expected output_text for assistant, got %v", asstContent["type"])
	}

	// Verify second user message has type: input_text
	userMsg2 := inputList[2].(map[string]any)
	userContent2 := userMsg2["content"].([]any)[0].(map[string]any)
	if userContent2["type"] != "input_text" {
		t.Errorf("expected input_text for second user msg, got %v", userContent2["type"])
	}
}

func TestResponsesStreamTranslation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: response.created\ndata: {\"type\":\"response.created\"}\n\n")
		fmt.Fprintf(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi there! I'm\"}\n\n")
		fmt.Fprintf(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" Muse Spark\"}\n\n")
		fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n")
	}))
	defer upstream.Close()

	client := opencode.NewClient(upstream.URL, nil)
	router := handler.NewRouter(nil, nil, client, nil, nil, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	reqBody := `{"model":"muse-spark-1.3-contributor-free","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req, _ := http.NewRequest("POST", server.URL+"/api/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /api/chat/completions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	var chunks []string
	var fullText strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				chunks = append(chunks, "[DONE]")
				break
			}
			var obj struct {
				Object  string `json:"object"`
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &obj); err == nil {
				if obj.Object == "chat.completion.chunk" && len(obj.Choices) > 0 {
					fullText.WriteString(obj.Choices[0].Delta.Content)
					chunks = append(chunks, obj.Choices[0].Delta.Content)
				}
			}
		}
	}

	if fullText.String() != "Hi there! I'm Muse Spark" {
		t.Errorf("expected translated text \"Hi there! I'm Muse Spark\", got %q", fullText.String())
	}
	if len(chunks) == 0 || chunks[len(chunks)-1] != "[DONE]" {
		t.Errorf("expected [DONE] at the end, chunks=%v", chunks)
	}
}

	func TestAntigravityAccountsAndProvider(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-antigravity-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	agStore, _ := antigravity.NewAccountStore(tempDir)
	agClient := antigravity.NewClient(agStore)

	router := handler.NewRouter(ks, ms, nil, agClient, nil, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	client := server.Client()

	// 1. Initial accounts should be empty
	resp, err := client.Get(server.URL + "/api/providers/antigravity/accounts")
	if err != nil {
		t.Fatalf("GET accounts: %v", err)
	}
	var listResp struct {
		Accounts []antigravity.Account `json:"accounts"`
		Count    int                   `json:"count"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	if listResp.Count != 0 {
		t.Errorf("expected 0 accounts initially, got %d", listResp.Count)
	}

	// 2. Add an account manually (token/credential paste)
	addPayload, _ := json.Marshal(map[string]string{
		"email":        "test-user@gmail.com",
		"projectId":    "cloudaicompanion-test-123",
		"accessToken":  "ya29.fake-access-token",
		"refreshToken": "1//fake-refresh-token",
	})
	resp, err = client.Post(server.URL+"/api/providers/antigravity/accounts", "application/json", bytes.NewReader(addPayload))
	if err != nil {
		t.Fatalf("POST account: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}
	var createdAcc antigravity.Account
	_ = json.NewDecoder(resp.Body).Decode(&createdAcc)
	resp.Body.Close()

	if createdAcc.Email != "test-user@gmail.com" {
		t.Errorf("expected email 'test-user@gmail.com', got '%s'", createdAcc.Email)
	}
	if createdAcc.ProjectID != "cloudaicompanion-test-123" {
		t.Errorf("expected projectId 'cloudaicompanion-test-123', got '%s'", createdAcc.ProjectID)
	}
	if !createdAcc.IsActive {
		t.Errorf("expected newly created account to be active")
	}

	// 3. List accounts and verify masking
	resp, _ = client.Get(server.URL + "/api/providers/antigravity/accounts")
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	if listResp.Count != 1 {
		t.Errorf("expected 1 account in list, got %d", listResp.Count)
	}

	// 4. Toggle account active state
	togglePayload, _ := json.Marshal(map[string]bool{"isActive": false})
	req, _ := http.NewRequest("PUT", server.URL+"/api/providers/antigravity/accounts/"+createdAcc.ID, bytes.NewReader(togglePayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("toggle account: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for toggle, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	acc, ok := agStore.Get(createdAcc.ID)
	if !ok || acc.IsActive {
		t.Errorf("expected account to be inactive in store")
	}

	// 5. Test OAuth Auth URL generation
	resp, err = client.Get(server.URL + "/api/providers/antigravity/oauth/auth-url")
	if err != nil {
		t.Fatalf("GET auth-url: %v", err)
	}
	var oauthResp struct {
		AuthURL  string `json:"authUrl"`
		ClientID string `json:"clientId"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&oauthResp)
	resp.Body.Close()
	if !strings.Contains(oauthResp.AuthURL, "accounts.google.com") || oauthResp.ClientID != antigravity.ClientID() {
		t.Errorf("unexpected oauth authUrl: %+v", oauthResp)
	}

	// 6. Test Antigravity Models list
	resp, err = client.Get(server.URL + "/api/providers/antigravity/models")
	if err != nil {
		t.Fatalf("GET models: %v", err)
	}
	var modelsResp struct {
		Provider string              `json:"provider"`
		Models   []handler.ModelItem `json:"models"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&modelsResp)
	resp.Body.Close()
	if modelsResp.Provider != "antigravity" || len(modelsResp.Models) == 0 {
		t.Errorf("unexpected models response: %+v", modelsResp)
	}

	// 7. Toggle Antigravity model enabled state
	modelTogglePayload, _ := json.Marshal(map[string]any{
		"model":   "gemini-3.8-flash",
		"enabled": false,
	})
	req, _ = http.NewRequest("PUT", server.URL+"/api/providers/antigravity/models", bytes.NewReader(modelTogglePayload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for model disable, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if !ms.IsDisabled("antigravity", "gemini-3.8-flash") {
		t.Errorf("expected gemini-3.8-flash to be disabled in modelStore")
	}

	// 8. Delete account
	req, _ = http.NewRequest("DELETE", server.URL+"/api/providers/antigravity/accounts/"+createdAcc.ID, nil)
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if _, ok := agStore.Get(createdAcc.ID); ok {
		t.Errorf("expected account to be deleted from store")
	}
}

func TestAntigravityQuotaTracker(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-router-quota-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	agStore, _ := antigravity.NewAccountStore(tempDir)
	agClient := antigravity.NewClient(agStore)

	router := handler.NewRouter(ks, ms, nil, agClient, nil, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	client := server.Client()

	// Seed two accounts: acc1 and acc2
	acc1 := &antigravity.Account{
		ID:        "acc_1",
		Email:     "acc1@gmail.com",
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	acc2 := &antigravity.Account{
		ID:        "acc_2",
		Email:     "acc2@gmail.com",
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	_ = agStore.Save(acc1)
	_ = agStore.Save(acc2)

	// Check IsExhausted initially false
	if agClient.IsExhausted(acc1.ID, "gemini-3.8-flash") {
		t.Errorf("expected acc1 to not be exhausted initially")
	}

	// Mark acc1 exhausted for gemini
	agClient.MarkExhausted(acc1.ID, "gemini-3.8-flash", time.Now().Add(10*time.Minute))
	if !agClient.IsExhausted(acc1.ID, "gemini-3.8-flash") {
		t.Errorf("expected acc1 to be marked exhausted")
	}

	// GetActiveForModel should pick acc2 since acc1 is exhausted for gemini
	chosen, ok := agClient.GetActiveForModel("gemini-3.8-flash")
	if !ok || chosen.ID != acc2.ID {
		t.Errorf("expected GetActiveForModel to select unexhausted acc2, got: %+v", chosen)
	}

	// Test GET /api/providers/antigravity/quota
	req, _ := http.NewRequest("GET", server.URL+"/api/providers/antigravity/quota", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to get quotas: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /api/providers/antigravity/quota, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test GET /api/providers/antigravity/accounts/nonexistent/quota returns 404
	req, _ = http.NewRequest("GET", server.URL+"/api/providers/antigravity/accounts/nonexistent/quota", nil)
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent account quota, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRTKWorkflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-rtk-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	rtkSvc, err := rtk.NewService(tempDir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	router := handler.NewRouter(ks, ms, nil, nil, rtkSvc, nil)
	server := httptest.NewServer(router)
	defer server.Close()

	client := server.Client()

	// 1. GET /api/rtk status
	resp, err := client.Get(server.URL + "/api/rtk")
	if err != nil {
		t.Fatalf("GET /api/rtk: %v", err)
	}
	var statusResp struct {
		RTKEnabled bool           `json:"rtk_enabled"`
		Stats      map[string]any `json:"stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("decode rtk status: %v", err)
	}
	resp.Body.Close()

	if !statusResp.RTKEnabled {
		t.Errorf("expected rtk_enabled to be true initially")
	}

	// Also verify backward compatibility alias /api/tokensaver
	compatResp, err := client.Get(server.URL + "/api/tokensaver")
	if err != nil || compatResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for backward compat /api/tokensaver")
	}
	compatResp.Body.Close()

	// 2. POST /api/rtk/test simulation
	longDiff := strings.Join([]string{
		"diff --git a/app.js b/app.js",
		"index 123..456 100644",
		"--- a/app.js",
		"+++ b/app.js",
		"@@ -1,3 +1,150 @@",
	}, "\n")
	for i := 0; i < 150; i++ {
		longDiff += fmt.Sprintf("\n+line %d %s", i, strings.Repeat("a", 25))
	}

	testPayload, _ := json.Marshal(map[string]string{"text": longDiff})
	resp, err = client.Post(server.URL+"/api/rtk/test", "application/json", bytes.NewReader(testPayload))
	if err != nil {
		t.Fatalf("POST /api/rtk/test: %v", err)
	}
	var testResp struct {
		Filter       string  `json:"filter"`
		OriginalSize int     `json:"original_size"`
		SavedBytes   int     `json:"saved_bytes"`
		SavedPercent float64 `json:"saved_percent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&testResp); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	resp.Body.Close()

	if testResp.Filter != "git-diff" {
		t.Errorf("expected filter git-diff, got %s", testResp.Filter)
	}
	if testResp.SavedBytes <= 0 {
		t.Errorf("expected saved_bytes > 0, got %d", testResp.SavedBytes)
	}

	// 3. PUT /api/rtk toggle off
	togglePayload, _ := json.Marshal(map[string]any{"rtk_enabled": false})
	req, _ := http.NewRequest("PUT", server.URL+"/api/rtk", bytes.NewReader(togglePayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/rtk: %v", err)
	}
	var updateResp struct {
		RTKEnabled bool `json:"rtk_enabled"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&updateResp)
	resp.Body.Close()

	if updateResp.RTKEnabled {
		t.Errorf("expected rtk_enabled to be false after toggle")
	}

	// 4. PUT /api/rtk toggle back on
	togglePayload, _ = json.Marshal(map[string]any{"rtk_enabled": true})
	req, _ = http.NewRequest("PUT", server.URL+"/api/rtk", bytes.NewReader(togglePayload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	_ = json.NewDecoder(resp.Body).Decode(&updateResp)
	resp.Body.Close()

	if !updateResp.RTKEnabled {
		t.Errorf("expected rtk_enabled to be true after second toggle")
	}
}

func TestCavemanWorkflow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-caveman-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ks, _ := store.NewKeyStore(tempDir)
	ms, _ := store.NewModelStore(tempDir)
	cavemanSvc, err := caveman.NewService(tempDir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer cavemanSvc.Close()

	router := handler.NewRouter(ks, ms, nil, nil, nil, cavemanSvc)
	server := httptest.NewServer(router)
	defer server.Close()

	client := server.Client()

	// 1. GET /api/caveman status
	resp, err := client.Get(server.URL + "/api/caveman")
	if err != nil {
		t.Fatalf("GET /api/caveman: %v", err)
	}
	var statusResp caveman.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("decode caveman status: %v", err)
	}
	resp.Body.Close()

	if !statusResp.Enabled {
		t.Errorf("expected enabled to be true initially")
	}
	if statusResp.Mode != "caveman" {
		t.Errorf("expected mode caveman, got %s", statusResp.Mode)
	}
	if len(statusResp.AvailableModes) < 3 {
		t.Errorf("expected at least 3 available modes")
	}

	// 2. POST /api/caveman/test
	testPayload, _ := json.Marshal(map[string]string{
		"mode":        "ultracave",
		"user_prompt": "Why is React re-rendering constantly?",
	})
	resp, err = client.Post(server.URL+"/api/caveman/test", "application/json", bytes.NewReader(testPayload))
	if err != nil {
		t.Fatalf("POST /api/caveman/test: %v", err)
	}
	var testResult struct {
		Applied        bool   `json:"applied"`
		Mode           string `json:"mode"`
		InjectedPrompt string `json:"injected_prompt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&testResult); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	resp.Body.Close()

	if !testResult.Applied {
		t.Errorf("expected test prompt to be applied")
	}
	if !strings.Contains(testResult.InjectedPrompt, "Ultracave is caveman") {
		t.Errorf("expected ultracave prompt in injected output")
	}

	// 3. PUT /api/caveman update config
	newMode := "ultracave"
	updatePayload, _ := json.Marshal(map[string]any{
		"mode": newMode,
	})
	req, _ := http.NewRequest("PUT", server.URL+"/api/caveman", bytes.NewReader(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/caveman: %v", err)
	}
	var updated caveman.StatusResponse
	_ = json.NewDecoder(resp.Body).Decode(&updated)
	resp.Body.Close()

	if updated.Mode != "ultracave" {
		t.Errorf("expected updated mode to be ultracave, got %s", updated.Mode)
	}
}
