package kilocode

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

type AccountStore struct {
	mu       sync.RWMutex
	filePath string
	accounts map[string]*Account
	rotIndex int
}

func NewAccountStore(dataDir string) (*AccountStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	fp := filepath.Join(dataDir, "kilocode_accounts.json")
	as := &AccountStore{
		filePath: fp,
		accounts: make(map[string]*Account),
	}

	if err := as.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load kilocode accounts: %w", err)
	}

	return as, nil
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

	s.accounts = make(map[string]*Account)
	for _, acc := range list {
		s.accounts[acc.ID] = acc
	}
	return nil
}

func (s *AccountStore) saveLocked() error {
	var list []*Account
	for _, acc := range s.accounts {
		list = append(list, acc)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, s.filePath)
}

func (s *AccountStore) List() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		safe := *acc
		if len(safe.AccessToken) > 12 {
			safe.AccessToken = safe.AccessToken[:6] + "..." + safe.AccessToken[len(safe.AccessToken)-4:]
		}
		res = append(res, safe)
	}
	return res
}

func (s *AccountStore) ListRaw() []*Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		cpy := *acc
		res = append(res, &cpy)
	}
	return res
}

func (s *AccountStore) ListActive() []*Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var active []*Account
	for _, acc := range s.accounts {
		if acc.IsActive {
			cpy := *acc
			active = append(active, &cpy)
		}
	}
	return active
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

func (s *AccountStore) GetActive() (*Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var active []*Account
	for _, acc := range s.accounts {
		if acc.IsActive {
			active = append(active, acc)
		}
	}

	if len(active) == 0 {
		return nil, false
	}

	s.rotIndex = (s.rotIndex + 1) % len(active)
	chosen := active[s.rotIndex]
	cpy := *chosen
	return &cpy, true
}

func (s *AccountStore) Save(acc *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if acc.ID == "" {
		acc.ID = generateAccountID()
	}
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = time.Now().UTC()
	}

	// Update existing account by matching ID, or matching AccessToken / Email
	foundExisting := false
	for _, existing := range s.accounts {
		if existing.ID == acc.ID || (acc.AccessToken != "" && existing.AccessToken == acc.AccessToken) {
			existing.Email = acc.Email
			existing.OrgID = acc.OrgID
			existing.AccessToken = acc.AccessToken
			existing.IsActive = acc.IsActive
			acc.ID = existing.ID
			foundExisting = true
			break
		}
	}

	if !foundExisting {
		s.accounts[acc.ID] = acc
	}

	return s.saveLocked()
}

func (s *AccountStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.accounts[id]; !exists {
		return fmt.Errorf("account not found: %s", id)
	}

	delete(s.accounts, id)
	return s.saveLocked()
}

func (s *AccountStore) SetActive(id string, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc, exists := s.accounts[id]
	if !exists {
		return fmt.Errorf("account not found: %s", id)
	}

	acc.IsActive = active
	return s.saveLocked()
}

func (s *AccountStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.accounts)
}

func (s *AccountStore) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, acc := range s.accounts {
		if acc.IsActive {
			n++
		}
	}
	return n
}

func generateAccountID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("kc-%s", hex.EncodeToString(b))
}
