package addons

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewPermissionStore(test *testing.T) {
	store := NewPermissionStore("")
	if store == nil {
		test.Fatal("NewPermissionStore returned nil")
	}
	if len(store.permissions) != 0 {
		test.Errorf("expected empty permissions, got %d", len(store.permissions))
	}
}

func TestPermissionStoreSetAndGet(test *testing.T) {
	store := NewPermissionStore("")

	store.Set("shell_exec", PermDeny)
	if store.Get("shell_exec") != PermDeny {
		test.Errorf("expected PermDeny for shell_exec, got %v", store.Get("shell_exec"))
	}

	store.Set("vault_read", PermAllow)
	if store.Get("vault_read") != PermAllow {
		test.Errorf("expected PermAllow for vault_read, got %v", store.Get("vault_read"))
	}

	store.Set("file_write", PermAsk)
	if store.Get("file_write") != PermAsk {
		test.Errorf("expected PermAsk for file_write, got %v", store.Get("file_write"))
	}
}

func TestPermissionStoreGlobPatternMatching(test *testing.T) {
	store := NewPermissionStore("")

	// Prefix glob
	store.Set("shell_*", PermDeny)
	if store.Get("shell_exec") != PermDeny {
		test.Errorf("expected shell_exec to match shell_* glob, got %v", store.Get("shell_exec"))
	}
	if store.Get("shell_cd") != PermDeny {
		test.Errorf("expected shell_cd to match shell_* glob, got %v", store.Get("shell_cd"))
	}

	// Suffix glob
	store.Set("*_read", PermAllow)
	if store.Get("vault_read") != PermAllow {
		test.Errorf("expected vault_read to match *_read glob, got %v", store.Get("vault_read"))
	}

	// Wildcard all
	store2 := NewPermissionStore("")
	store2.Set("*", PermAsk)
	if store2.Get("anything") != PermAsk {
		test.Errorf("expected * glob to match anything, got %v", store2.Get("anything"))
	}
}

func TestPermissionStoreDefaultPermissionForUnknownTools(test *testing.T) {
	store := NewPermissionStore("")

	// Read-like tools should default to allow
	if store.Get("vault_list") != PermAllow {
		test.Errorf("expected read-like tool default to PermAllow, got %v", store.Get("vault_list"))
	}
	if store.Get("search_files") != PermAllow {
		test.Errorf("expected search tool default to PermAllow, got %v", store.Get("search_files"))
	}

	// Write-like tools should default to ask
	if store.Get("file_write") != PermAsk {
		test.Errorf("expected write tool default to PermAsk, got %v", store.Get("file_write"))
	}
	if store.Get("shell_exec") != PermAsk {
		test.Errorf("expected exec tool default to PermAsk, got %v", store.Get("shell_exec"))
	}

	// Delete-like tools should default to ask
	if store.Get("file_delete") != PermAsk {
		test.Errorf("expected delete tool default to PermAsk, got %v", store.Get("file_delete"))
	}
}

func TestPermissionStoreRemove(test *testing.T) {
	store := NewPermissionStore("")
	store.Set("test_tool", PermDeny)

	if store.Get("test_tool") != PermDeny {
		test.Fatal("precondition: test_tool should be deny")
	}

	store.Remove("test_tool")

	// After removal, should fall back to default
	result := store.Get("test_tool")
	if result == PermDeny {
		test.Error("expected default permission after removal, still got PermDeny")
	}
}

func TestPermissionStoreAll(test *testing.T) {
	store := NewPermissionStore("")
	store.Set("tool_a", PermAllow)
	store.Set("tool_b", PermDeny)

	all := store.All()
	if len(all) != 2 {
		test.Fatalf("expected 2 permissions, got %d", len(all))
	}
	if all["tool_a"] != PermAllow {
		test.Errorf("expected tool_a = PermAllow, got %v", all["tool_a"])
	}
	if all["tool_b"] != PermDeny {
		test.Errorf("expected tool_b = PermDeny, got %v", all["tool_b"])
	}
}

func TestPermissionStoreFormat(test *testing.T) {
	store := NewPermissionStore("")

	// Empty store
	result := store.Format()
	if !strings.Contains(result, "Keine expliziten") {
		test.Errorf("expected empty format message, got: %s", result)
	}

	// With entries
	store.Set("shell_exec", PermDeny)
	store.Set("vault_read", PermAllow)
	result = store.Format()
	if !strings.Contains(result, "Tool Permissions") {
		test.Errorf("expected 'Tool Permissions' in format, got: %s", result)
	}
	if !strings.Contains(result, "shell_exec") {
		test.Errorf("expected shell_exec in format, got: %s", result)
	}
}

func TestPermissionStorePersistence(test *testing.T) {
	tmpDir := test.TempDir()
	filePath := filepath.Join(tmpDir, "permissions.yaml")

	// Create and save
	store1 := NewPermissionStore(filePath)
	store1.Set("tool_a", PermAllow)
	store1.Set("tool_b", PermDeny)

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		test.Fatal("expected permissions file to be created")
	}

	// Load from file
	store2 := NewPermissionStore(filePath)
	if store2.Get("tool_a") != PermAllow {
		test.Errorf("expected tool_a = PermAllow after reload, got %v", store2.Get("tool_a"))
	}
	if store2.Get("tool_b") != PermDeny {
		test.Errorf("expected tool_b = PermDeny after reload, got %v", store2.Get("tool_b"))
	}
}

func TestPermissionStringRepresentation(test *testing.T) {
	if PermAsk.String() != "ask" {
		test.Errorf("expected 'ask', got %q", PermAsk.String())
	}
	if PermAllow.String() != "allow" {
		test.Errorf("expected 'allow', got %q", PermAllow.String())
	}
	if PermDeny.String() != "deny" {
		test.Errorf("expected 'deny', got %q", PermDeny.String())
	}
}

func TestParsePermission(test *testing.T) {
	cases := map[string]ToolPermission{
		"allow": PermAllow,
		"yes":   PermAllow,
		"ja":    PermAllow,
		"deny":  PermDeny,
		"no":    PermDeny,
		"nein":  PermDeny,
		"block": PermDeny,
		"ask":   PermAsk,
		"prompt": PermAsk,
	}
	for input, expected := range cases {
		result, ok := parsePermission(input)
		if !ok {
			test.Errorf("parsePermission(%q) returned not ok", input)
			continue
		}
		if result != expected {
			test.Errorf("parsePermission(%q) = %v, expected %v", input, result, expected)
		}
	}

	// Unknown
	_, ok := parsePermission("unknown")
	if ok {
		test.Error("expected parsePermission('unknown') to return not ok")
	}
}

func TestMatchGlob(test *testing.T) {
	cases := []struct {
		pattern string
		name    string
		match   bool
	}{
		{"*", "anything", true},
		{"shell_*", "shell_exec", true},
		{"shell_*", "vault_read", false},
		{"*_read", "vault_read", true},
		{"*_read", "vault_write", false},
		{"exact", "exact", true},
		{"exact", "other", false},
	}
	for _, tc := range cases {
		result := matchGlob(tc.pattern, tc.name)
		if result != tc.match {
			test.Errorf("matchGlob(%q, %q) = %v, expected %v", tc.pattern, tc.name, result, tc.match)
		}
	}
}
