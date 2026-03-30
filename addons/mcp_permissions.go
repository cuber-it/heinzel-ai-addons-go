// MCP tool permission system — allow, deny, ask per tool with glob patterns.

package addons

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Permission levels for MCP tools
type ToolPermission int

const (
	PermAsk   ToolPermission = iota // ask user before execution (default for write/delete)
	PermAllow                       // always allow
	PermDeny                        // always deny
)

var permNames = [...]string{"ask", "allow", "deny"}

func (perm ToolPermission) String() string {
	if int(perm) < len(permNames) {
		return permNames[perm]
	}
	return "unknown"
}

func parsePermission(s string) (ToolPermission, bool) {
	switch strings.ToLower(s) {
	case "allow", "yes", "ja":
		return PermAllow, true
	case "deny", "no", "nein", "block":
		return PermDeny, true
	case "ask", "prompt":
		return PermAsk, true
	}
	return PermAsk, false
}

// PermissionStore manages tool permissions with persistence
type PermissionStore struct {
	mu          sync.RWMutex
	permissions map[string]ToolPermission // tool name → permission
	filePath    string
}

func NewPermissionStore(filePath string) *PermissionStore {
	store := &PermissionStore{
		permissions: make(map[string]ToolPermission),
		filePath:    filePath,
	}
	store.load()
	return store
}

func (store *PermissionStore) Get(toolName string) ToolPermission {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if perm, ok := store.permissions[toolName]; ok {
		return perm
	}

	for pattern, perm := range store.permissions {
		if strings.Contains(pattern, "*") {
			if matchGlob(pattern, toolName) {
				return perm
			}
		}
	}

	return store.defaultPermission(toolName)
}

func (store *PermissionStore) Set(pattern string, perm ToolPermission) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.permissions[pattern] = perm
	store.save()
}

func (store *PermissionStore) Remove(pattern string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.permissions, pattern)
	store.save()
}

func (store *PermissionStore) All() map[string]ToolPermission {
	store.mu.RLock()
	defer store.mu.RUnlock()
	result := make(map[string]ToolPermission, len(store.permissions))
	for k, v := range store.permissions {
		result[k] = v
	}
	return result
}

func (store *PermissionStore) Format() string {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.permissions) == 0 {
		return "Keine expliziten Permissions gesetzt.\nDefaults: read=allow, write/delete=ask"
	}

	var keys []string
	for k := range store.permissions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, key := range keys {
		perm := store.permissions[key]
		marker := "  "
		switch perm {
		case PermAllow:
			marker = "✓ "
		case PermDeny:
			marker = "✗ "
		case PermAsk:
			marker = "? "
		}
		lines = append(lines, fmt.Sprintf("  %s%-30s %s", marker, key, perm))
	}
	return "Tool Permissions:\n" + strings.Join(lines, "\n")
}

// Smart defaults: read operations -> allow, write/delete/exec -> ask
func (store *PermissionStore) defaultPermission(toolName string) ToolPermission {
	lower := strings.ToLower(toolName)

	denyWords := []string{"delete", "remove", "drop", "destroy", "kill", "format"}
	for _, word := range denyWords {
		if strings.Contains(lower, word) {
			return PermAsk // not deny — ask first
		}
	}

	askWords := []string{"write", "exec", "run", "create", "update", "modify",
		"set", "put", "post", "send", "deploy", "install"}
	for _, word := range askWords {
		if strings.Contains(lower, word) {
			return PermAsk
		}
	}

	return PermAllow
}

func (store *PermissionStore) load() {
	if store.filePath == "" {
		return
	}
	data, err := os.ReadFile(store.filePath)
	if err != nil {
		return
	}

	var raw struct {
		Tools map[string]string `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return
	}

	for name, permStr := range raw.Tools {
		if perm, ok := parsePermission(permStr); ok {
			store.permissions[name] = perm
		}
	}
}

func (store *PermissionStore) save() {
	if store.filePath == "" {
		return
	}

	raw := struct {
		Tools map[string]string `yaml:"tools"`
	}{
		Tools: make(map[string]string),
	}
	for name, perm := range store.permissions {
		raw.Tools[name] = perm.String()
	}

	data, err := yaml.Marshal(&raw)
	if err != nil {
		return
	}

	os.MkdirAll(filepath.Dir(store.filePath), 0755)
	os.WriteFile(store.filePath, data, 0644)
}

func matchGlob(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(name, suffix)
	}
	return pattern == name
}
