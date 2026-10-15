package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Config stores irreversible password hash and metadata on disk
type Config struct {
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Service manages password authentication and session tokens
type Service struct {
	mu         sync.RWMutex
	filePath   string
	config     Config
	sessions   map[string]time.Time
	sessionTTL time.Duration
}

// NewService loads or initializes auth configuration from data directory
func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create auth data dir: %w", err)
	}

	svc := &Service{
		filePath:   filepath.Join(dataDir, "auth.json"),
		sessions:   make(map[string]time.Time),
		sessionTTL: 30 * 24 * time.Hour, // 30 days
	}

	if data, err := os.ReadFile(svc.filePath); err == nil {
		if err := json.Unmarshal(data, &svc.config); err != nil {
			return nil, fmt.Errorf("unmarshal auth config: %w", err)
		}
	} else if os.IsNotExist(err) {
		// Auto-initialize from environment if provided
		envPass := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
		if envPass == "" {
			envPass = strings.TrimSpace(os.Getenv("LUN_PASSWORD"))
		}
		if envPass != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(envPass), bcrypt.DefaultCost)
			if err != nil {
				return nil, fmt.Errorf("hash env password: %w", err)
			}
			svc.config = Config{
				PasswordHash: string(hash),
				CreatedAt:    time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			}
			if err := svc.saveLocked(); err != nil {
				return nil, fmt.Errorf("save initial auth: %w", err)
			}
		}
	} else {
		return nil, fmt.Errorf("read auth file: %w", err)
	}

	return svc, nil
}

// IsAuthEnabled returns true if a password has been set
func (s *Service) IsAuthEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.PasswordHash != ""
}

// SetupPassword sets initial password if none is configured
func (s *Service) SetupPassword(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.PasswordHash != "" {
		return errors.New("password already configured")
	}

	password = strings.TrimSpace(password)
	if len(password) < 4 {
		return errors.New("password must be at least 4 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	s.config = Config{
		PasswordHash: string(hash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	return s.saveLocked()
}

// VerifyPassword checks password against irreversible bcrypt hash
func (s *Service) VerifyPassword(password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.config.PasswordHash == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(s.config.PasswordHash), []byte(password))
	return err == nil
}

// ChangePassword updates existing password after validating current password
func (s *Service) ChangePassword(currentPassword, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.PasswordHash == "" {
		return errors.New("auth not configured")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(s.config.PasswordHash), []byte(currentPassword)); err != nil {
		return errors.New("invalid current password")
	}

	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 4 {
		return errors.New("new password must be at least 4 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	s.config.PasswordHash = string(hash)
	s.config.UpdatedAt = time.Now().UTC()

	return s.saveLocked()
}

// CreateSession generates a secure random 64-character session token
func (s *Service) CreateSession() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = time.Now().Add(s.sessionTTL)
	return token, nil
}

// ValidateSession verifies session token validity and expiration
func (s *Service) ValidateSession(token string) bool {
	if token == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	expiry, exists := s.sessions[token]
	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		delete(s.sessions, token)
		return false
	}

	return true
}

// RevokeSession invalidates a session token
func (s *Service) RevokeSession(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *Service) saveLocked() error {
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth config: %w", err)
	}
	return os.WriteFile(s.filePath, data, 0600)
}
