package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lun-router/internal/model"
)

// KeyStore manages thread-safe JSON-persisted keys.
// ponytail: file-backed JSON store handles up to ~5k keys; upgrade to modernc.org/sqlite when query filtering or concurrency demands ACID transactions.
type KeyStore struct {
	mu        sync.RWMutex
	filePath  string
	machineID string
	secret    string
	keys      []model.APIKey
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
		keys:      []model.APIKey{},
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
	dir := filepath.Dir(ks.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

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

func (ks *KeyStore) MachineID() string {
	return ks.machineID
}

func (ks *KeyStore) List() []model.APIKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	result := make([]model.APIKey, len(ks.keys))
	copy(result, ks.keys)
	return result
}

func (ks *KeyStore) Get(id string) (model.APIKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	for _, k := range ks.keys {
		if k.ID == id {
			return k, true
		}
	}
	return model.APIKey{}, false
}

func (ks *KeyStore) FindByKey(rawKey string) (model.APIKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	for _, k := range ks.keys {
		if k.Key == rawKey {
			return k, true
		}
	}
	return model.APIKey{}, false
}

func (ks *KeyStore) Create(name string, customKey string) (model.APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.APIKey{}, errors.New("name is required")
	}

	customKey = strings.TrimSpace(customKey)
	var finalKey string

	if customKey != "" {
		finalKey = customKey
	} else {
		finalKey = generate9RouterKey(ks.machineID, ks.secret)
	}

	id := generateRandomHex(16)
	newKey := model.APIKey{
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
		return model.APIKey{}, fmt.Errorf("save key: %w", err)
	}

	return newKey, nil
}

func (ks *KeyStore) Update(id string, req model.UpdateKeyRequest) (model.APIKey, bool, error) {
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
				return model.APIKey{}, true, fmt.Errorf("save updated key: %w", err)
			}
			return ks.keys[i], true, nil
		}
	}

	return model.APIKey{}, false, nil
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
