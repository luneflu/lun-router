package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
