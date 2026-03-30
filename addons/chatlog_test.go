package addons

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewChatLogAddon(test *testing.T) {
	addon := NewChatLogAddon("")
	if addon == nil {
		test.Fatal("NewChatLogAddon returned nil")
	}
	if addon.Name() != "chatlog" {
		test.Errorf("expected name 'chatlog', got %q", addon.Name())
	}
	if addon.Type() != core.AddonObserver {
		test.Errorf("expected type AddonObserver, got %v", addon.Type())
	}
}

func TestChatLogAddonCreatesLogFileOnSessionEnd(test *testing.T) {
	logDir := test.TempDir()
	addon := NewChatLogAddon(logDir)
	addon.Start()

	ctx := testContext()

	// Add some log entries
	ctx.Log.Log("user", "Hello", "test")
	ctx.Log.Log("assistant", "Hi there", "test")

	// Trigger session end
	addon.Handle(core.OnSessionEnd, ctx)

	// Check that a file was created
	entries, err := os.ReadDir(logDir)
	if err != nil {
		test.Fatalf("failed to read log dir: %v", err)
	}
	if len(entries) != 1 {
		test.Fatalf("expected 1 log file, got %d", len(entries))
	}

	logFile := entries[0].Name()
	if !strings.HasSuffix(logFile, ".json") {
		test.Errorf("expected .json extension, got: %s", logFile)
	}
}

func TestChatLogAddonLogContainsMessagesAsJSON(test *testing.T) {
	logDir := test.TempDir()
	addon := NewChatLogAddon(logDir)
	addon.Start()

	ctx := testContext()
	ctx.Log.Log("user", "Wie geht es?", "test")
	ctx.Log.Log("assistant", "Mir geht es gut!", "test")

	addon.Handle(core.OnSessionEnd, ctx)

	// Read the created log file
	entries, _ := os.ReadDir(logDir)
	if len(entries) == 0 {
		test.Fatal("no log file created")
	}

	data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		test.Fatalf("failed to read log file: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		test.Fatalf("log file is not valid JSON: %v", err)
	}

	// Should contain entries
	logEntries, ok := parsed["entries"]
	if !ok {
		test.Fatal("expected 'entries' key in log JSON")
	}
	entryList, ok := logEntries.([]interface{})
	if !ok {
		test.Fatal("expected entries to be an array")
	}
	if len(entryList) != 2 {
		test.Errorf("expected 2 log entries, got %d", len(entryList))
	}
}

func TestChatLogAddonNoFileIfNoMessages(test *testing.T) {
	logDir := test.TempDir()
	addon := NewChatLogAddon(logDir)
	addon.Start()

	ctx := testContext()
	// No log entries added

	addon.Handle(core.OnSessionEnd, ctx)

	entries, _ := os.ReadDir(logDir)
	if len(entries) != 0 {
		test.Errorf("expected no log file when no messages, got %d files", len(entries))
	}
}

func TestChatLogAddonHandleCommandStats(test *testing.T) {
	addon := NewChatLogAddon("")
	ctx := testContext()
	ctx.Log.Log("user", "test", "test")

	result := addon.HandleCommand("log", "stats", ctx)
	if !strings.Contains(result, "Log entries: 1") {
		test.Errorf("expected stats with 1 entry, got: %s", result)
	}
}
