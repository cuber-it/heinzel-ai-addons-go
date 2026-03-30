// Authentication, sessions, TOTP, and OAuth for Heinzel.
package addons

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	sessionCookieName = "heinzel_session"
	sessionTimeout    = 24 * time.Hour
	tokenLength       = 32 // bytes -> 64 hex chars
)

type AuthConfig struct {
	Mode         string `json:"mode" yaml:"mode"`                   // "passkey", "password", "local"
	PasswordHash string `json:"password_hash" yaml:"password_hash"` // bcrypt hash
	TOTPSecret   string `json:"totp_secret" yaml:"totp_secret"`     // base32-encoded TOTP secret
}

type Session struct {
	Token      string    `json:"token"`
	Created    time.Time `json:"created"`
	LastAccess time.Time `json:"last_access"`
	Expires    time.Time `json:"expires"`
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

func (store *SessionStore) Create() (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	now := time.Now()
	session := &Session{
		Token:      token,
		Created:    now,
		LastAccess: now,
		Expires:    now.Add(sessionTimeout),
	}

	store.mu.Lock()
	store.sessions[token] = session
	store.mu.Unlock()

	return token, nil
}

// Validate checks the token and refreshes LastAccess on success.
func (store *SessionStore) Validate(token string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	session, ok := store.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(session.Expires) {
		delete(store.sessions, token)
		return false
	}
	session.LastAccess = time.Now()
	return true
}

func (store *SessionStore) Delete(token string) {
	store.mu.Lock()
	delete(store.sessions, token)
	store.mu.Unlock()
}

func (store *SessionStore) Cleanup() {
	store.mu.Lock()
	defer store.mu.Unlock()

	now := time.Now()
	for token, session := range store.sessions {
		if now.After(session.Expires) {
			delete(store.sessions, token)
		}
	}
}

type AuthSystem struct {
	Config   AuthConfig
	Sessions *SessionStore
}

func NewAuthSystem(config AuthConfig) *AuthSystem {
	return &AuthSystem{
		Config:   config,
		Sessions: NewSessionStore(),
	}
}

// AuthMiddleware wraps an http.Handler with authentication checks.
//   - mode "local": always pass through
//   - requests from localhost (127.0.0.1, ::1): always pass through
//   - otherwise: require valid session cookie
func (auth *AuthSystem) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if auth.Config.Mode == "local" {
			next.ServeHTTP(writer, request)
			return
		}

	if isLocalhost(request) {
			next.ServeHTTP(writer, request)
			return
		}

	cookie, err := request.Cookie(sessionCookieName)
		if err != nil || !auth.Sessions.Validate(cookie.Value) {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

// LoginHandler handles POST /api/auth/login.
//
// Request body (JSON):
//
//	{"password": "...", "totp": "..."}
//
// On success sets a session cookie and returns 200.
func (auth *AuthSystem) LoginHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		http.Error(writer, "invalid request body", http.StatusBadRequest)
		return
	}

	switch auth.Config.Mode {
	case "password":
		if err := bcrypt.CompareHashAndPassword([]byte(auth.Config.PasswordHash), []byte(payload.Password)); err != nil {
			http.Error(writer, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if auth.Config.TOTPSecret != "" {
			if !ValidateTOTP(auth.Config.TOTPSecret, payload.TOTP) {
				http.Error(writer, "invalid TOTP code", http.StatusUnauthorized)
				return
			}
		}

	case "passkey":
		// TODO: Implement WebAuthn/passkey verification.
		// The go-webauthn library (github.com/go-webauthn/webauthn) needs
		// careful integration — credential storage, challenge management, etc.
		http.Error(writer, "passkey auth not yet implemented", http.StatusNotImplemented)
		return

	case "local":
		// Middleware lets everything through; handle gracefully if reached anyway.

	default:
		http.Error(writer, "unknown auth mode", http.StatusInternalServerError)
		return
	}

	token, err := auth.Sessions.Create()
	if err != nil {
		http.Error(writer, "session error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTimeout.Seconds()),
	})

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}

// LogoutHandler handles POST /api/auth/logout.
func (auth *AuthSystem) LogoutHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "POST only", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := request.Cookie(sessionCookieName)
	if err == nil {
		auth.Sessions.Delete(cookie.Value)
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}

// --- TOTP helpers ---

func GenerateTOTP(accountName string) (secret string, provisioningURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Heinzel",
		AccountName: accountName,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func ValidateTOTP(secret string, code string) bool {
	return totp.Validate(code, secret)
}

// --- Password helpers ---

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// --- Internal helpers ---

func generateToken() (string, error) {
	bytes := make([]byte, tokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func isLocalhost(request *http.Request) bool {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1"
}
