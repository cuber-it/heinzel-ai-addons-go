package addons

import (
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestNewAuthSystem(test *testing.T) {
	auth := NewAuthSystem(AuthConfig{Mode: "password"})
	if auth == nil {
		test.Fatal("NewAuthSystem returned nil")
	}
	if auth.Config.Mode != "password" {
		test.Errorf("expected mode 'password', got %q", auth.Config.Mode)
	}
	if auth.Sessions == nil {
		test.Fatal("expected Sessions to be initialized")
	}
}

func TestHashPasswordAndVerify(test *testing.T) {
	password := "test-password-123"
	hash, err := HashPassword(password)
	if err != nil {
		test.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		test.Fatal("expected non-empty hash")
	}
	if hash == password {
		test.Error("hash should not equal plaintext password")
	}

	// Verify correct password
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		test.Errorf("correct password should verify, got error: %v", err)
	}

	// Verify wrong password
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong-password"))
	if err == nil {
		test.Error("wrong password should not verify")
	}
}

func TestSessionStoreCreateAndValidate(test *testing.T) {
	store := NewSessionStore()
	if store == nil {
		test.Fatal("NewSessionStore returned nil")
	}

	token, err := store.Create()
	if err != nil {
		test.Fatalf("Create failed: %v", err)
	}
	if token == "" {
		test.Fatal("expected non-empty token")
	}
	if len(token) != 64 { // 32 bytes = 64 hex chars
		test.Errorf("expected token length 64, got %d", len(token))
	}

	// Validate
	if !store.Validate(token) {
		test.Error("expected valid token to validate")
	}

	// Invalid token
	if store.Validate("invalid-token") {
		test.Error("expected invalid token to fail validation")
	}
}

func TestSessionStoreDelete(test *testing.T) {
	store := NewSessionStore()
	token, _ := store.Create()

	if !store.Validate(token) {
		test.Fatal("precondition: token should be valid")
	}

	store.Delete(token)

	if store.Validate(token) {
		test.Error("expected deleted token to fail validation")
	}
}

func TestSessionStoreExpiry(test *testing.T) {
	store := NewSessionStore()
	token, _ := store.Create()

	// Manually set expiry to the past
	store.mu.Lock()
	store.sessions[token].Expires = time.Now().Add(-1 * time.Hour)
	store.mu.Unlock()

	if store.Validate(token) {
		test.Error("expected expired token to fail validation")
	}

	// Token should be removed after failed validation
	store.mu.RLock()
	_, exists := store.sessions[token]
	store.mu.RUnlock()
	if exists {
		test.Error("expected expired token to be removed from store")
	}
}

func TestSessionStoreCleanup(test *testing.T) {
	store := NewSessionStore()
	token1, _ := store.Create()
	token2, _ := store.Create()

	// Expire token1
	store.mu.Lock()
	store.sessions[token1].Expires = time.Now().Add(-1 * time.Hour)
	store.mu.Unlock()

	store.Cleanup()

	store.mu.RLock()
	_, exists1 := store.sessions[token1]
	_, exists2 := store.sessions[token2]
	store.mu.RUnlock()

	if exists1 {
		test.Error("expected expired token1 to be cleaned up")
	}
	if !exists2 {
		test.Error("expected valid token2 to survive cleanup")
	}
}

func TestSessionStoreMultipleSessions(test *testing.T) {
	store := NewSessionStore()

	token1, _ := store.Create()
	token2, _ := store.Create()

	if token1 == token2 {
		test.Error("expected unique tokens for different sessions")
	}

	if !store.Validate(token1) || !store.Validate(token2) {
		test.Error("expected both tokens to be valid")
	}
}

func TestIsLocalhost(test *testing.T) {
	cases := []struct {
		remoteAddr string
		expected   bool
	}{
		{"127.0.0.1:8080", true},
		{"[::1]:8080", true},
		{"192.168.1.100:8080", false},
		{"10.0.0.1:3000", false},
	}

	for _, tc := range cases {
		request := &http.Request{RemoteAddr: tc.remoteAddr}
		result := isLocalhost(request)
		if result != tc.expected {
			test.Errorf("isLocalhost(%q) = %v, expected %v", tc.remoteAddr, result, tc.expected)
		}
	}
}

func TestAuthSystemLocalModeConfig(test *testing.T) {
	auth := NewAuthSystem(AuthConfig{Mode: "local"})
	if auth.Config.Mode != "local" {
		test.Errorf("expected mode 'local', got %q", auth.Config.Mode)
	}
}

func TestSessionValidateRefreshesLastAccess(test *testing.T) {
	store := NewSessionStore()
	token, _ := store.Create()

	// Record initial last access
	store.mu.RLock()
	initialAccess := store.sessions[token].LastAccess
	store.mu.RUnlock()

	// Wait a tiny bit then validate
	time.Sleep(1 * time.Millisecond)
	store.Validate(token)

	store.mu.RLock()
	updatedAccess := store.sessions[token].LastAccess
	store.mu.RUnlock()

	if !updatedAccess.After(initialAccess) {
		test.Error("expected LastAccess to be updated after Validate")
	}
}
