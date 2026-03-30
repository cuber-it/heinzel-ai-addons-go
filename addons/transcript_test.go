package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewTranscriptAddon(test *testing.T) {
	addon := NewTranscriptAddon("")
	if addon == nil {
		test.Fatal("NewTranscriptAddon returned nil")
	}
	if addon.Name() != "transcript" {
		test.Errorf("expected name 'transcript', got %q", addon.Name())
	}
	if addon.Type() != core.AddonObserver {
		test.Errorf("expected type AddonObserver, got %v", addon.Type())
	}
}

func TestTranscriptAddonHooks(test *testing.T) {
	addon := NewTranscriptAddon("")
	hooks := addon.Hooks()
	if len(hooks) != 4 {
		test.Fatalf("expected 4 hooks, got %d", len(hooks))
	}
}

func TestTranscriptAddonRecordsTurnsOnOutput(test *testing.T) {
	dir := test.TempDir()
	addon := NewTranscriptAddon(dir)
	ctx := testContext()

	// Start session to open file
	addon.Handle(core.OnSessionStart, ctx)

	// Simulate a turn
	ctx.Input = "Hallo Heinzel"
	addon.Handle(core.OnInput, ctx)
	ctx.Output = "Hallo! Wie kann ich helfen?"
	addon.Handle(core.OnOutput, ctx)

	if addon.counter != 1 {
		test.Errorf("expected counter 1, got %d", addon.counter)
	}

	// Check that turn number was stored in context
	val, ok := ctx.Get(KeyLastTurnNumber)
	if !ok {
		test.Fatal("expected KeyLastTurnNumber to be set")
	}
	if val.(int) != 1 {
		test.Errorf("expected turn number 1, got %v", val)
	}
}

func TestTranscriptAddonSearchFindsMatchingTurns(test *testing.T) {
	dir := test.TempDir()
	addon := NewTranscriptAddon(dir)
	ctx := testContext()

	addon.Handle(core.OnSessionStart, ctx)

	// Record two turns
	ctx.Input = "Was ist Go?"
	addon.Handle(core.OnInput, ctx)
	ctx.Output = "Go ist eine Programmiersprache."
	addon.Handle(core.OnOutput, ctx)

	ctx.Input = "Was ist Rust?"
	addon.Handle(core.OnInput, ctx)
	ctx.Output = "Rust ist eine systemnahe Sprache."
	addon.Handle(core.OnOutput, ctx)

	// Search for "Go"
	result := addon.searchTranscript("Go")
	if strings.Contains(result, "Keine Turns") {
		test.Errorf("expected to find turns with 'Go', got: %s", result)
	}
	if !strings.Contains(result, "Gefunden") {
		test.Errorf("expected 'Gefunden' in search result, got: %s", result)
	}

	// Search for something non-existent
	result = addon.searchTranscript("Python")
	if !strings.Contains(result, "Keine Turns") {
		test.Errorf("expected no results for 'Python', got: %s", result)
	}
}

func TestTranscriptAddonHandleCommandRecallLast(test *testing.T) {
	dir := test.TempDir()
	addon := NewTranscriptAddon(dir)
	ctx := testContext()

	addon.Handle(core.OnSessionStart, ctx)

	// Record a turn
	ctx.Input = "Test input"
	addon.Handle(core.OnInput, ctx)
	ctx.Output = "Test output"
	addon.Handle(core.OnOutput, ctx)

	result := addon.HandleCommand("recall", "last", ctx)
	if strings.Contains(result, "No turns") {
		test.Errorf("expected turn data, got: %s", result)
	}
	if !strings.Contains(result, "Test input") {
		test.Errorf("expected 'Test input' in last turns, got: %s", result)
	}
}

func TestTranscriptAddonHandleCommandNoArgs(test *testing.T) {
	addon := NewTranscriptAddon("")
	ctx := testContext()

	result := addon.HandleCommand("recall", "", ctx)
	if !strings.Contains(result, "Transcript:") {
		test.Errorf("expected transcript stats, got: %s", result)
	}
}

func TestTranscriptAddonSessionEndClosesFile(test *testing.T) {
	dir := test.TempDir()
	addon := NewTranscriptAddon(dir)
	ctx := testContext()

	addon.Handle(core.OnSessionStart, ctx)
	addon.Handle(core.OnSessionEnd, ctx)

	if addon.file != nil {
		test.Error("expected file to be nil after session end")
	}
}
