package customprovider

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	prefixRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	reservedPrefixes = map[string]bool{
		"opencode":    true,
		"antigravity": true,
		"chatgpt":     true,
		"zai":         true,
		"kilocode":    true,
		"kilo":        true,
		"kc":          true,
	}
)

type Store struct {
	mu       sync.RWMutex
	filePath string
	items    map[string]Provider // keyed by ID
}

func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir dataDir: %w", err)
	}

	fp := filepath.Join(dataDir, "custom_providers.json")
	s := &Store{
		filePath: fp,
		items:    make(map[string]Provider),
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load custom providers: %w", err)
	}

	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var list []Provider
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.items = make(map[string]Provider)
	for _, p := range list {
		if p.Models == nil {
			p.Models = []string{}
		}
		s.items[p.ID] = p
	}
	return nil
}

func (s *Store) saveLocked() error {
	list := make([]Provider, 0, len(s.items))
	for _, p := range s.items {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

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

func ValidatePrefix(prefix string) error {
	clean := strings.ToLower(strings.TrimSpace(prefix))
	if clean == "" {
		return errors.New("provider prefix is required")
	}
	if reservedPrefixes[clean] {
		return fmt.Errorf("prefix %q is reserved for built-in providers", clean)
	}
	if !prefixRegex.MatchString(clean) {
		return fmt.Errorf("prefix %q is invalid: must start with alphanumeric and contain only lowercase letters, digits, hyphen or underscore", clean)
	}
	return nil
}

func ValidateBaseURL(rawURL string) (string, error) {
	clean := strings.TrimSpace(rawURL)
	if clean == "" {
		return "", errors.New("base URL is required")
	}
	u, err := url.Parse(clean)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("base URL %q is invalid: must start with http:// or https://", clean)
	}
	clean = strings.TrimRight(clean, "/")
	return clean, nil
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "cp_" + hex.EncodeToString(b)
}

// List returns all custom providers ordered by creation time.
func (s *Store) List() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]Provider, 0, len(s.items))
	for _, p := range s.items {
		res = append(res, p)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].CreatedAt.Before(res[j].CreatedAt)
	})
	return res
}

// Get returns provider by ID.
func (s *Store) Get(id string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.items[id]
	return p, ok
}

// GetByPrefix returns provider by its namespace prefix.
func (s *Store) GetByPrefix(prefix string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clean := strings.ToLower(strings.TrimSpace(prefix))
	for _, p := range s.items {
		if strings.ToLower(p.Prefix) == clean {
			return p, true
		}
	}
	return Provider{}, false
}

// FindByModel searches if any custom provider has this bare model in its catalog.
func (s *Store) FindByModel(modelID string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clean := strings.TrimSpace(modelID)
	for _, p := range s.items {
		if !p.IsEnabled {
			continue
		}
		for _, m := range p.Models {
			if m == clean {
				return p, true
			}
		}
	}
	return Provider{}, false
}

// Add creates a new custom provider.
func (s *Store) Add(p Provider) (Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Provider{}, errors.New("provider name is required")
	}

	p.Prefix = strings.ToLower(strings.TrimSpace(p.Prefix))
	if err := ValidatePrefix(p.Prefix); err != nil {
		return Provider{}, err
	}

	for _, existing := range s.items {
		if strings.ToLower(existing.Prefix) == p.Prefix {
			return Provider{}, fmt.Errorf("provider with prefix %q already exists", p.Prefix)
		}
	}

	baseURL, err := ValidateBaseURL(p.BaseURL)
	if err != nil {
		return Provider{}, err
	}
	p.BaseURL = baseURL

	p.APIKey = strings.TrimSpace(p.APIKey)

	if p.ID == "" {
		p.ID = generateID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Models == nil {
		p.Models = []string{}
	}

	s.items[p.ID] = p
	if err := s.saveLocked(); err != nil {
		delete(s.items, p.ID)
		return Provider{}, err
	}
	return p, nil
}

// Update modifies an existing custom provider.
func (s *Store) Update(id string, updateFn func(p *Provider) error) (Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.items[id]
	if !ok {
		return Provider{}, errors.New("provider not found")
	}

	orig := existing
	if err := updateFn(&existing); err != nil {
		return Provider{}, err
	}

	existing.Name = strings.TrimSpace(existing.Name)
	if existing.Name == "" {
		return Provider{}, errors.New("provider name is required")
	}

	existing.Prefix = strings.ToLower(strings.TrimSpace(existing.Prefix))
	if err := ValidatePrefix(existing.Prefix); err != nil {
		return Provider{}, err
	}

	for _, other := range s.items {
		if other.ID != id && strings.ToLower(other.Prefix) == existing.Prefix {
			return Provider{}, fmt.Errorf("provider with prefix %q already exists", existing.Prefix)
		}
	}

	baseURL, err := ValidateBaseURL(existing.BaseURL)
	if err != nil {
		return Provider{}, err
	}
	existing.BaseURL = baseURL
	existing.UpdatedAt = time.Now()

	s.items[id] = existing
	if err := s.saveLocked(); err != nil {
		s.items[id] = orig
		return Provider{}, err
	}
	return existing, nil
}

// Delete removes a provider by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.items[id]; !ok {
		return errors.New("provider not found")
	}
	delete(s.items, id)
	return s.saveLocked()
}

// SetEnabled toggles provider enabled state.
func (s *Store) SetEnabled(id string, enabled bool) error {
	_, err := s.Update(id, func(p *Provider) error {
		p.IsEnabled = enabled
		return nil
	})
	return err
}

// SetModels updates models list for a provider.
func (s *Store) SetModels(id string, models []string) error {
	_, err := s.Update(id, func(p *Provider) error {
		unique := make([]string, 0, len(models))
		seen := make(map[string]bool)
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m != "" && !seen[m] {
				seen[m] = true
				unique = append(unique, m)
			}
		}
		sort.Strings(unique)
		p.Models = unique
		return nil
	})
	return err
}
