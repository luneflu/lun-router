package opencode

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateSessionID generates canonical OpenCode session: ses_<12 hex><14 base62>
// Fix for GitHub Issue #4781 (Bypass HTTP 403 FreeTierError)
func GenerateSessionID() string {
	nowMs := time.Now().UnixMilli()
	val := (^uint64(nowMs*4096 + 1)) & 0xFFFFFFFFFFFF
	timePart := fmt.Sprintf("%012x", val)

	randomBytes := make([]byte, 14)
	_, _ = rand.Read(randomBytes)
	var randomPart strings.Builder
	for _, b := range randomBytes {
		randomPart.WriteByte(base62Chars[int(b)%62])
	}

	return fmt.Sprintf("ses_%s%s", timePart, randomPart.String())
}

type SessionEntry struct {
	SessionID string
	LastUsed  time.Time
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*SessionEntry
	ttl      time.Duration
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &SessionManager{
		sessions: make(map[string]*SessionEntry),
		ttl:      ttl,
	}
}

// Get returns current stable session for identity or mints a new one
func (sm *SessionManager) Get(identity string) string {
	if identity == "" {
		identity = "default"
	}

	sm.mu.RLock()
	entry, ok := sm.sessions[identity]
	if ok && time.Since(entry.LastUsed) < sm.ttl {
		sm.mu.RUnlock()
		sm.mu.Lock()
		entry.LastUsed = time.Now()
		sm.mu.Unlock()
		return entry.SessionID
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double check under write lock
	if entry, ok := sm.sessions[identity]; ok && time.Since(entry.LastUsed) < sm.ttl {
		entry.LastUsed = time.Now()
		return entry.SessionID
	}

	newID := GenerateSessionID()
	sm.sessions[identity] = &SessionEntry{
		SessionID: newID,
		LastUsed:  time.Now(),
	}
	return newID
}

// Evict drops session for identity
func (sm *SessionManager) Evict(identity string) {
	if identity == "" {
		identity = "default"
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, identity)
}
