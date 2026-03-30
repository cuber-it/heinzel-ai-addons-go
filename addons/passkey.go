// Passkey (WebAuthn) authentication for Heinzel Assistant.
//
// Provides registration and login flows using the go-webauthn library.
// Credentials are stored as JSON in ~/.heinzel/auth/passkey.json.
package addons

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-webauthn/webauthn/webauthn"
)

type PasskeyConfig struct {
	RPID          string   `json:"rp_id" yaml:"rp_id"`                   // e.g. "localhost"
	RPDisplayName string   `json:"rp_display_name" yaml:"rp_display_name"` // e.g. "Heinzel Assistant"
	RPOrigins     []string `json:"rp_origins" yaml:"rp_origins"`           // e.g. ["http://localhost:12080"]
}

type PasskeyUser struct {
	UserID      []byte                `json:"user_id"`
	UserName    string                `json:"user_name"`
	DisplayName string                `json:"display_name"`
	Credentials []webauthn.Credential `json:"credentials"`
}

func (user *PasskeyUser) WebAuthnID() []byte                         { return user.UserID }
func (user *PasskeyUser) WebAuthnName() string                      { return user.UserName }
func (user *PasskeyUser) WebAuthnDisplayName() string               { return user.DisplayName }
func (user *PasskeyUser) WebAuthnCredentials() []webauthn.Credential { return user.Credentials }

type PasskeyStore struct {
	mu       sync.RWMutex
	filePath string
	user     *PasskeyUser
}

func NewPasskeyStore(filePath string) (*PasskeyStore, error) {
	store := &PasskeyStore{filePath: filePath}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			store.user = &PasskeyUser{
				UserID:      []byte("heinzel-owner"),
				UserName:    "owner",
				DisplayName: "Heinzel Owner",
			}
			return store, nil
		}
		return nil, fmt.Errorf("reading passkey store: %w", err)
	}

	var user PasskeyUser
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("parsing passkey store: %w", err)
	}
	store.user = &user
	return store, nil
}

func (store *PasskeyStore) User() *PasskeyUser {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.user
}

func (store *PasskeyStore) AddCredential(credential webauthn.Credential) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.user.Credentials = append(store.user.Credentials, credential)
	return store.save()
}

func (store *PasskeyStore) UpdateCredential(credential webauthn.Credential) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	for index := range store.user.Credentials {
		if string(store.user.Credentials[index].ID) == string(credential.ID) {
			store.user.Credentials[index] = credential
			return store.save()
		}
	}
	return fmt.Errorf("credential not found")
}

func (store *PasskeyStore) save() error {
	dir := filepath.Dir(store.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating auth directory: %w", err)
	}

	data, err := json.MarshalIndent(store.user, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling passkey store: %w", err)
	}
	return os.WriteFile(store.filePath, data, 0600)
}

type PasskeyHandler struct {
	webAuthn     *webauthn.WebAuthn
	store        *PasskeyStore
	sessions     *SessionStore
	sessionCache sync.Map // challenge -> *webauthn.SessionData (temporary, per-ceremony)
}

func NewPasskeyHandler(config PasskeyConfig, store *PasskeyStore, sessions *SessionStore) (*PasskeyHandler, error) {
	webAuthn, err := webauthn.New(&webauthn.Config{
		RPID:          config.RPID,
		RPDisplayName: config.RPDisplayName,
		RPOrigins:     config.RPOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing webauthn: %w", err)
	}

	return &PasskeyHandler{
		webAuthn: webAuthn,
		store:    store,
		sessions: sessions,
	}, nil
}

func (handler *PasskeyHandler) RegisterBeginHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "GET only", http.StatusMethodNotAllowed)
		return
	}

	user := handler.store.User()

	creation, sessionData, err := handler.webAuthn.BeginRegistration(user)
	if err != nil {
		http.Error(writer, fmt.Sprintf("begin registration: %v", err), http.StatusInternalServerError)
		return
	}

	handler.sessionCache.Store("register", sessionData)

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(creation)
}

func (handler *PasskeyHandler) RegisterFinishHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "POST only", http.StatusMethodNotAllowed)
		return
	}

	sessionDataValue, ok := handler.sessionCache.LoadAndDelete("register")
	if !ok {
		http.Error(writer, "no registration in progress", http.StatusBadRequest)
		return
	}
	sessionData := sessionDataValue.(*webauthn.SessionData)

	user := handler.store.User()

	credential, err := handler.webAuthn.FinishRegistration(user, *sessionData, request)
	if err != nil {
		http.Error(writer, fmt.Sprintf("finish registration: %v", err), http.StatusBadRequest)
		return
	}

	if err := handler.store.AddCredential(*credential); err != nil {
		http.Error(writer, fmt.Sprintf("storing credential: %v", err), http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}

func (handler *PasskeyHandler) LoginBeginHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "GET only", http.StatusMethodNotAllowed)
		return
	}

	user := handler.store.User()
	if len(user.Credentials) == 0 {
		http.Error(writer, "no passkeys registered", http.StatusPreconditionFailed)
		return
	}

	assertion, sessionData, err := handler.webAuthn.BeginLogin(user)
	if err != nil {
		http.Error(writer, fmt.Sprintf("begin login: %v", err), http.StatusInternalServerError)
		return
	}

	handler.sessionCache.Store("login", sessionData)

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(assertion)
}

func (handler *PasskeyHandler) LoginFinishHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "POST only", http.StatusMethodNotAllowed)
		return
	}

	sessionDataValue, ok := handler.sessionCache.LoadAndDelete("login")
	if !ok {
		http.Error(writer, "no login in progress", http.StatusBadRequest)
		return
	}
	sessionData := sessionDataValue.(*webauthn.SessionData)

	user := handler.store.User()

	credential, err := handler.webAuthn.FinishLogin(user, *sessionData, request)
	if err != nil {
		http.Error(writer, fmt.Sprintf("finish login: %v", err), http.StatusUnauthorized)
		return
	}

	if err := handler.store.UpdateCredential(*credential); err != nil {
		fmt.Fprintf(os.Stderr, "warning: updating credential after login: %v\n", err)
	}

	token, err := handler.sessions.Create()
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
