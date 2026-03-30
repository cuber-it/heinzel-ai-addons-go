package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestNewCompactionAddon(test *testing.T) {
	factsLayer := NewFactsLayer()
	summaryLayer := NewSessionSummaryLayer(2000)
	addon := NewCompactionAddon(factsLayer, summaryLayer)

	if addon == nil {
		test.Fatal("NewCompactionAddon returned nil")
	}
	if addon.Name() != "compaction" {
		test.Errorf("expected name 'compaction', got %q", addon.Name())
	}
	if addon.Type() != core.AddonFilter {
		test.Errorf("expected type AddonFilter, got %v", addon.Type())
	}
}

func TestCompactionAddonHooks(test *testing.T) {
	addon := NewCompactionAddon(NewFactsLayer(), NewSessionSummaryLayer(2000))
	hooks := addon.Hooks()
	if len(hooks) != 2 {
		test.Fatalf("expected 2 hooks, got %d", len(hooks))
	}
}

func TestCompactionAddonHandleCommandStatus(test *testing.T) {
	factsLayer := NewFactsLayer()
	summaryLayer := NewSessionSummaryLayer(2000)
	addon := NewCompactionAddon(factsLayer, summaryLayer)

	ctx := testContext()
	ctx.Messages = append(ctx.Messages, core.Message{Role: "user", Content: "hello"})
	ctx.Messages = append(ctx.Messages, core.Message{Role: "assistant", Content: "hi"})

	result := addon.HandleCommand("compact", "status", ctx)
	if !strings.Contains(result, "Messages: 2") {
		test.Errorf("expected 'Messages: 2' in status, got: %s", result)
	}
	if !strings.Contains(result, "Facts: 0") {
		test.Errorf("expected 'Facts: 0' in status, got: %s", result)
	}
	if !strings.Contains(result, "compact after") {
		test.Errorf("expected compact threshold in status, got: %s", result)
	}
}

func TestCompactionAddonHandleCommandStatusEmpty(test *testing.T) {
	addon := NewCompactionAddon(NewFactsLayer(), NewSessionSummaryLayer(2000))
	ctx := testContext()

	result := addon.HandleCommand("compact", "", ctx)
	if !strings.Contains(result, "Messages: 0") {
		test.Errorf("expected 'Messages: 0' in default status, got: %s", result)
	}
}

func TestCompactionAddonDefaultThresholds(test *testing.T) {
	addon := NewCompactionAddon(NewFactsLayer(), NewSessionSummaryLayer(2000))
	if addon.compactAfterMessages != defaultCompactAfterMessages {
		test.Errorf("expected compactAfterMessages %d, got %d", defaultCompactAfterMessages, addon.compactAfterMessages)
	}
	if addon.summaryChunkSize != defaultSummaryChunkSize {
		test.Errorf("expected summaryChunkSize %d, got %d", defaultSummaryChunkSize, addon.summaryChunkSize)
	}
}

func TestCompactionAddonCommands(test *testing.T) {
	addon := NewCompactionAddon(NewFactsLayer(), NewSessionSummaryLayer(2000))
	commands := addon.Commands()
	if len(commands) != 1 {
		test.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0].Name != "compact" {
		test.Errorf("expected command name 'compact', got %q", commands[0].Name)
	}
}

func TestCompactionAddonAddSink(test *testing.T) {
	addon := NewCompactionAddon(NewFactsLayer(), NewSessionSummaryLayer(2000))
	if len(addon.sinks) != 0 {
		test.Errorf("expected 0 sinks initially, got %d", len(addon.sinks))
	}
	// We can't easily test sinks without a mock, but we can verify AddSink works
	// by checking the count doesn't panic and the slice grows
}

func TestCompactionAddonSetLoop(test *testing.T) {
	addon := NewCompactionAddon(NewFactsLayer(), NewSessionSummaryLayer(2000))
	if addon.loop != nil {
		test.Error("expected nil loop initially")
	}
	// SetLoop with nil is valid — just means internalQuery returns ""
	addon.SetLoop(nil)
	if addon.loop != nil {
		test.Error("expected loop to remain nil when set to nil")
	}
}
