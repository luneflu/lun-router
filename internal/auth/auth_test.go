package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthService(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lun_auth_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	svc, err := NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	// 1. Initial state: not enabled
	if svc.IsAuthEnabled() {
		t.Fatal("expected auth to be disabled initially")
	}

	// 2. Reject short password
	if err := svc.SetupPassword("12"); err == nil {
		t.Fatal("expected error for password < 4 chars")
	}

	// 3. Setup valid password
	password := "superSecret123!"
	if err := svc.SetupPassword(password); err != nil {
		t.Fatalf("SetupPassword failed: %v", err)
	}

	if !svc.IsAuthEnabled() {
		t.Fatal("expected auth to be enabled after setup")
	}

	// 4. Verify password hash is bcrypt format and non-reversible
	if !strings.HasPrefix(svc.config.PasswordHash, "$2a$") && !strings.HasPrefix(svc.config.PasswordHash, "$2b$") {
		t.Fatalf("expected bcrypt hash prefix, got %s", svc.config.PasswordHash)
	}
	if strings.Contains(svc.config.PasswordHash, password) {
		t.Fatal("password hash must not contain plaintext password")
	}

	// 5. Verify password matching
	if !svc.VerifyPassword(password) {
		t.Fatal("expected VerifyPassword to succeed with correct password")
	}
	if svc.VerifyPassword("wrongPassword") {
		t.Fatal("expected VerifyPassword to fail with incorrect password")
	}

	// 6. Test session generation and validation
	token, err := svc.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("expected 64-char hex token, got length %d", len(token))
	}
	if !svc.ValidateSession(token) {
		t.Fatal("expected ValidateSession to succeed for new token")
	}

	// 7. Test session revocation
	svc.RevokeSession(token)
	if svc.ValidateSession(token) {
		t.Fatal("expected ValidateSession to fail after revocation")
	}

	// 8. Test ChangePassword
	newPassword := "brandNewSecret456!"
	if err := svc.ChangePassword("wrongCurrent", newPassword); err == nil {
		t.Fatal("expected error on invalid current password")
	}
	if err := svc.ChangePassword(password, newPassword); err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}
	if !svc.VerifyPassword(newPassword) {
		t.Fatal("expected VerifyPassword to succeed with new password")
	}
	if svc.VerifyPassword(password) {
		t.Fatal("expected VerifyPassword to fail with old password")
	}

	// 9. Verify persistence on new service instance
	svc2, err := NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService re-read failed: %v", err)
	}
	if !svc2.IsAuthEnabled() {
		t.Fatal("expected persisted auth to be enabled on reload")
	}
	if !svc2.VerifyPassword(newPassword) {
		t.Fatal("expected persisted password to verify on reload")
	}
}

func TestAuthEnvAutoInit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lun_auth_env_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("ADMIN_PASSWORD", "envAdminPass999")
	defer os.Unsetenv("ADMIN_PASSWORD")

	svc, err := NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	if !svc.IsAuthEnabled() {
		t.Fatal("expected auth to be enabled from env var")
	}
	if !svc.VerifyPassword("envAdminPass999") {
		t.Fatal("expected password from env var to verify")
	}

	// File should exist
	if _, err := os.Stat(filepath.Join(tmpDir, "auth.json")); err != nil {
		t.Fatalf("expected auth.json to exist: %v", err)
	}
}

func TestSessionPersistsAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()

	svc, err := NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}
	if err := svc.SetupPassword("superSecret123!"); err != nil {
		t.Fatalf("SetupPassword failed: %v", err)
	}
	token, err := svc.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	restarted, err := NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService after restart error: %v", err)
	}
	if !restarted.ValidateSession(token) {
		t.Fatal("expected session to survive a service restart")
	}

	restarted.RevokeSession(token)
	again, err := NewService(tmpDir)
	if err != nil {
		t.Fatalf("NewService after revoke error: %v", err)
	}
	if again.ValidateSession(token) {
		t.Fatal("expected revoked session to stay revoked after restart")
	}
}
