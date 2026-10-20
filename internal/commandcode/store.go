package commandcode

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AccountStore persists Command Code API keys in data/commandcode_accounts.json.
type AccountStore struct {
	mu       sync.RWMutex
	filePath string
	accounts map[string]*Account
}

func NewAccountStore(dataDir string) (*AccountStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	s := &AccountStore{
		filePath: filepath.Join(dataDir, "commandcode_accounts.json"),
		accounts: make(map[string]*Account),
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load commandcode accounts: %w", err)
	}
	return s, nil
}

func (s *AccountStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var list []*Account
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	s.accounts = make(map[string]*Account, len(list))
	for _, acc := range list {
		s.accounts[acc.ID] = acc
	}
	return nil
}

func (s *AccountStore) saveLocked() error {
	list := make([]*Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		list = append(list, acc)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

// List returns accounts with the key masked, safe for the provider API.
func (s *AccountStore) List() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		safe := *acc
		if len(safe.APIKey) > 12 {
			safe.APIKey = safe.APIKey[:8] + "..." + safe.APIKey[len(safe.APIKey)-4:]
		} else {
			safe.APIKey = ""
		}
		res = append(res, safe)
	}
	return res
}

func (s *AccountStore) Get(id string) (*Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, ok := s.accounts[id]
	if !ok {
		return nil, false
	}
	cpy := *acc
	return &cpy, true
}

// GetActive rotates across active keys so a rate-limited key is not the only one used.
func (s *AccountStore) GetActive() (*Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ids []string
	for id, acc := range s.accounts {
		if acc.IsActive {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, false
	}

	chosen := s.accounts[ids[time.Now().UnixNano()%int64(len(ids))]]
	cpy := *chosen
	return &cpy, true
}

func (s *AccountStore) Save(acc *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if acc.ID == "" {
		acc.ID = newID("cc")
	}
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = time.Now().UTC()
	}

	for _, existing := range s.accounts {
		if existing.ID == acc.ID || (acc.APIKey != "" && existing.APIKey == acc.APIKey) {
			existing.Name = acc.Name
			existing.APIKey = acc.APIKey
			existing.IsActive = acc.IsActive
			acc.ID = existing.ID
			return s.saveLocked()
		}
	}

	s.accounts[acc.ID] = acc
	return s.saveLocked()
}

func (s *AccountStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.accounts[id]; !ok {
		return fmt.Errorf("account not found: %s", id)
	}
	delete(s.accounts, id)
	return s.saveLocked()
}

func (s *AccountStore) SetActive(id string, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc, ok := s.accounts[id]
	if !ok {
		return fmt.Errorf("account not found: %s", id)
	}
	acc.IsActive = active
	return s.saveLocked()
}

func newID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
