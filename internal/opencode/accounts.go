package opencode

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Account is one OpenCode Console OAuth login. The access token authenticates
// against the /zen/go/v1 OpenCode Go endpoints with x-opencode-org-id.
type Account struct {
	ID           string    `json:"id"`
	Email        string    `json:"email,omitempty"`
	Server       string    `json:"server"`
	OrgID        string    `json:"orgId,omitempty"`
	OrgName      string    `json:"orgName,omitempty"`
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	IsActive     bool      `json:"isActive"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// AccountStore persists OpenCode Console logins in data/opencode_accounts.json.
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
		filePath: filepath.Join(dataDir, "opencode_accounts.json"),
		accounts: make(map[string]*Account),
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load opencode accounts: %w", err)
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
		if acc.ID == "" {
			acc.ID = newAccountID()
		}
		if acc.CreatedAt.IsZero() {
			acc.CreatedAt = time.Now().UTC()
		}
		if acc.UpdatedAt.IsZero() {
			acc.UpdatedAt = acc.CreatedAt
		}
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
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

// List returns accounts with tokens masked, safe for the provider API.
func (s *AccountStore) List() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		safe := *acc
		safe.AccessToken = maskToken(safe.AccessToken)
		safe.RefreshToken = maskToken(safe.RefreshToken)
		res = append(res, safe)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].CreatedAt.Before(res[j].CreatedAt) })
	return res
}

func maskToken(token string) string {
	if len(token) > 12 {
		return token[:8] + "..." + token[len(token)-4:]
	}
	if token == "" {
		return ""
	}
	return "••••"
}

// ListRaw returns full accounts including tokens, for internal quota fetches.
func (s *AccountStore) ListRaw() []*Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*Account, 0, len(s.accounts))
	for _, acc := range s.accounts {
		cpy := *acc
		res = append(res, &cpy)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].CreatedAt.Before(res[j].CreatedAt) })
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

// SelectForSession picks one active account deterministically from the session
// identity. The same conversation always stays on the same login, so accounts
// never switch mid-session; nil means anonymous.
func (s *AccountStore) SelectForSession(sessionKey string) *Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ids []string
	for id, acc := range s.accounts {
		if acc.IsActive && acc.AccessToken != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	sort.Strings(ids)
	if sessionKey == "" {
		sessionKey = "default"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(sessionKey))
	chosen := s.accounts[ids[int(h.Sum32()%uint32(len(ids)))]]
	cpy := *chosen
	return &cpy
}

// FirstActive returns any active account, used for account-agnostic calls.
func (s *AccountStore) FirstActive() *Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ids []string
	for id, acc := range s.accounts {
		if acc.IsActive && acc.AccessToken != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	cpy := *s.accounts[ids[0]]
	return &cpy
}

func (s *AccountStore) Save(acc *Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if acc.ID == "" {
		acc.ID = newAccountID()
	}
	now := time.Now().UTC()
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = now
	}
	acc.UpdatedAt = now

	for _, existing := range s.accounts {
		if existing.ID == acc.ID || (acc.Email != "" && existing.Email == acc.Email && existing.Server == acc.Server) {
			existing.Email = acc.Email
			existing.Server = acc.Server
			existing.OrgID = acc.OrgID
			existing.OrgName = acc.OrgName
			existing.AccessToken = acc.AccessToken
			existing.RefreshToken = acc.RefreshToken
			existing.ExpiresAt = acc.ExpiresAt
			existing.IsActive = acc.IsActive
			existing.UpdatedAt = now
			acc.ID = existing.ID
			return s.saveLocked()
		}
	}

	s.accounts[acc.ID] = acc
	return s.saveLocked()
}

// UpdateTokens persists refreshed OAuth credentials.
func (s *AccountStore) UpdateTokens(id, access, refresh string, expires time.Time, orgID, orgName string) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc, ok := s.accounts[id]
	if !ok {
		return nil, fmt.Errorf("account not found: %s", id)
	}
	acc.AccessToken = access
	if refresh != "" {
		acc.RefreshToken = refresh
	}
	acc.ExpiresAt = expires
	if orgID != "" {
		acc.OrgID = orgID
	}
	if orgName != "" {
		acc.OrgName = orgName
	}
	acc.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	cpy := *acc
	return &cpy, nil
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
	acc.UpdatedAt = time.Now().UTC()
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
		if acc.IsActive && acc.AccessToken != "" {
			n++
		}
	}
	return n
}

func newAccountID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "oc-" + hex.EncodeToString(b)
}

// HasSessionAccounts reports whether at least one OAuth login is available.
func (s *AccountStore) HasSessionAccounts() bool {
	return s.ActiveCount() > 0
}
