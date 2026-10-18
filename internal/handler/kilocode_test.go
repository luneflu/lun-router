package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"lun-router/internal/kilocode"
	"lun-router/internal/store"
)

func TestKiloCodeHandler(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kilo-handler-test-*")
	if err != nil {
		t.Fatalf("tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	kcStore, _ := kilocode.NewAccountStore(tempDir)
	modelStore, _ := store.NewModelStore(tempDir)
	kcClient := kilocode.NewClient(kcStore, tempDir)

	kcH := NewKiloCodeHandler(kcClient, modelStore)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/providers/kilocode/accounts", kcH.ListAccounts)
	mux.HandleFunc("POST /api/providers/kilocode/accounts", kcH.CreateAccount)
	mux.HandleFunc("PUT /api/providers/kilocode/accounts/{id}", kcH.UpdateAccount)
	mux.HandleFunc("DELETE /api/providers/kilocode/accounts/{id}", kcH.DeleteAccount)
	mux.HandleFunc("GET /api/providers/kilocode/models", kcH.ListModels)
	mux.HandleFunc("PUT /api/providers/kilocode/models", kcH.UpdateModels)

	// 1. Initial accounts should be empty
	req := httptest.NewRequest("GET", "/api/providers/kilocode/accounts", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ListAccounts status %d", w.Code)
	}

	var accList struct {
		Accounts []kilocode.Account `json:"accounts"`
		Count    int                `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&accList); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	if accList.Count != 0 {
		t.Errorf("expected 0 accounts, got %d", accList.Count)
	}

	// 2. Models listing should include fallbacks
	req = httptest.NewRequest("GET", "/api/providers/kilocode/models", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ListModels status %d", w.Code)
	}

	var modelList struct {
		Models []ModelItem `json:"models"`
		Count  int         `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&modelList); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if len(modelList.Models) == 0 {
		t.Errorf("expected models, got 0")
	}

	// 3. Toggle model disabled
	toggleBody, _ := json.Marshal(map[string]any{
		"model":   modelList.Models[0].ID,
		"enabled": false,
	})
	req = httptest.NewRequest("PUT", "/api/providers/kilocode/models", bytes.NewReader(toggleBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("UpdateModels status %d", w.Code)
	}

	// Verify model is disabled
	req = httptest.NewRequest("GET", "/api/providers/kilocode/models", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	_ = json.NewDecoder(w.Body).Decode(&modelList)
	if modelList.Models[0].Enabled {
		t.Errorf("expected model %s to be disabled", modelList.Models[0].ID)
	}
}
