package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// APIKey represents a managed key.
type APIKey struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	MachineID string `json:"machineId,omitempty"`
	IsActive  bool   `json:"isActive"`
	CreatedAt string `json:"createdAt"`
}

type CreateKeyRequest struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"` // Optional custom key input
}

type UpdateKeyRequest struct {
	Name     *string `json:"name,omitempty"`
	IsActive *bool   `json:"isActive,omitempty"`
}

// KeyStore manages thread-safe JSON-persisted keys.
// ponytail: file-backed JSON store handles up to ~5k keys; upgrade to modernc.org/sqlite when query filtering or concurrency demands ACID transactions.
type KeyStore struct {
	mu        sync.RWMutex
	filePath  string
	machineID string
	secret    string
	keys      []APIKey
}

func NewKeyStore(dataDir string) (*KeyStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	machineID, err := getOrCreateMachineID(dataDir)
	if err != nil {
		return nil, fmt.Errorf("machine id: %w", err)
	}

	secret := os.Getenv("API_KEY_SECRET")
	if secret == "" {
		secret = "endpoint-proxy-api-key-secret"
	}

	ks := &KeyStore{
		filePath:  filepath.Join(dataDir, "keys.json"),
		machineID: machineID,
		secret:    secret,
		keys:      []APIKey{},
	}

	if err := ks.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load keys: %w", err)
	}

	return ks, nil
}

func (ks *KeyStore) load() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	data, err := os.ReadFile(ks.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &ks.keys)
}

func (ks *KeyStore) saveLocked() error {
	data, err := json.MarshalIndent(ks.keys, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := ks.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpFile, ks.filePath)
}

func (ks *KeyStore) List() []APIKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	result := make([]APIKey, len(ks.keys))
	copy(result, ks.keys)
	return result
}

func (ks *KeyStore) Get(id string) (APIKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	for _, k := range ks.keys {
		if k.ID == id {
			return k, true
		}
	}
	return APIKey{}, false
}

func (ks *KeyStore) FindByKey(rawKey string) (APIKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	for _, k := range ks.keys {
		if k.Key == rawKey {
			return k, true
		}
	}
	return APIKey{}, false
}

func (ks *KeyStore) Create(name string, customKey string) (APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return APIKey{}, errors.New("name is required")
	}

	customKey = strings.TrimSpace(customKey)
	var finalKey string

	if customKey != "" {
		finalKey = customKey
	} else {
		finalKey = generate9RouterKey(ks.machineID, ks.secret)
	}

	id := generateRandomHex(16)
	newKey := APIKey{
		ID:        id,
		Key:       finalKey,
		Name:      name,
		MachineID: ks.machineID,
		IsActive:  true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	ks.keys = append(ks.keys, newKey)
	if err := ks.saveLocked(); err != nil {
		return APIKey{}, fmt.Errorf("save key: %w", err)
	}

	return newKey, nil
}

func (ks *KeyStore) Update(id string, req UpdateKeyRequest) (APIKey, bool, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	for i, k := range ks.keys {
		if k.ID == id {
			if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
				ks.keys[i].Name = strings.TrimSpace(*req.Name)
			}
			if req.IsActive != nil {
				ks.keys[i].IsActive = *req.IsActive
			}
			if err := ks.saveLocked(); err != nil {
				return APIKey{}, true, fmt.Errorf("save updated key: %w", err)
			}
			return ks.keys[i], true, nil
		}
	}

	return APIKey{}, false, nil
}

func (ks *KeyStore) Delete(id string) (bool, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	for i, k := range ks.keys {
		if k.ID == id {
			ks.keys = append(ks.keys[:i], ks.keys[i+1:]...)
			if err := ks.saveLocked(); err != nil {
				return true, fmt.Errorf("save after delete: %w", err)
			}
			return true, nil
		}
	}

	return false, nil
}

// 9router key format: sk-{machineId}-{keyId}-{crc}
func generate9RouterKey(machineID, secret string) string {
	keyID := generateRandomAlphaNum(6)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(machineID + keyID))
	crc := hex.EncodeToString(h.Sum(nil))[:8]
	return fmt.Sprintf("sk-%s-%s-%s", machineID, keyID, crc)
}

func getOrCreateMachineID(dataDir string) (string, error) {
	idPath := filepath.Join(dataDir, "machine-id")
	if data, err := os.ReadFile(idPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return strings.TrimSpace(string(data)), nil
	}

	raw := generateRandomHex(16)
	hash := sha256.Sum256([]byte(raw + "endpoint-proxy-salt"))
	machineID := hex.EncodeToString(hash[:])[:16]

	_ = os.WriteFile(idPath, []byte(machineID), 0600)
	return machineID, nil
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

func generateRandomAlphaNum(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, n)
	maxVal := big.NewInt(int64(len(charset)))
	for i := range result {
		idx, _ := rand.Int(rand.Reader, maxVal)
		result[i] = charset[idx.Int64()]
	}
	return string(result)
}

// Router & Handlers
func setupRoutes(ks *KeyStore) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/keys", func(w http.ResponseWriter, r *http.Request) {
		keys := ks.List()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	})

	mux.HandleFunc("POST /api/keys", func(w http.ResponseWriter, r *http.Request) {
		var req CreateKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		key, err := ks.Create(req.Name, req.Key)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(key)
	})

	mux.HandleFunc("GET /api/keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		key, ok := ks.Get(id)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Key not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"key": key})
	})

	mux.HandleFunc("PUT /api/keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req UpdateKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		key, ok, err := ks.Update(id, req)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Key not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"key": key})
	})

	mux.HandleFunc("DELETE /api/keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ok, err := ks.Delete(id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Key not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Key deleted successfully"})
	})

	mux.HandleFunc("GET /api/keys/verify", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || token == authHeader {
			token = r.URL.Query().Get("key")
		}

		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "missing token"})
			return
		}

		key, ok := ks.FindByKey(token)
		if !ok || !key.IsActive {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "invalid or inactive key"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "key": key})
	})

	// Wrap in CORS middleware
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	ks, err := NewKeyStore(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize KeyStore: %v", err)
	}

	handler := setupRoutes(ks)

	log.Printf("Lun Router Go backend running on :%s (data dir: %s)", port, dataDir)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
