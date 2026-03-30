package addons

import (
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

func TestCommandAddonHandleCommandRoutes(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "/help"

	result := addon.Handle(core.OnInput, ctx)
	if !result.Halt {
		test.Error("expected /help to halt the pipeline")
	}
	if ctx.Output == "" {
		test.Error("expected help output, got empty")
	}
	if !strings.Contains(ctx.Output, "Commands") {
		test.Errorf("expected help to contain 'Commands', got: %s", ctx.Output[:80])
	}
}

func TestCommandAddonUnknownCommandNotHandled(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "/nonexistent"

	result := addon.Handle(core.OnInput, ctx)
	// Unknown commands that are not in handlers and not dispatched should not halt
	if result.Halt {
		test.Error("unknown command should not halt (unless dispatched)")
	}
}

func TestCommandAddonClearCommand(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Messages = append(ctx.Messages, core.Message{Role: "user", Content: "hello"})
	ctx.Messages = append(ctx.Messages, core.Message{Role: "assistant", Content: "hi"})
	ctx.MemoryResults["test"] = "value"

	ctx.Input = "/clear"
	result := addon.Handle(core.OnInput, ctx)

	if !result.Halt {
		test.Error("expected /clear to halt")
	}
	if len(ctx.Messages) != 0 {
		test.Errorf("expected messages cleared, got %d", len(ctx.Messages))
	}
	if len(ctx.MemoryResults) != 0 {
		test.Errorf("expected memory results cleared, got %d", len(ctx.MemoryResults))
	}
	if !strings.Contains(ctx.Output, "gelöscht") {
		test.Errorf("expected German clear message, got: %s", ctx.Output)
	}
}

func TestCommandAddonNameCommand(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "/name test-session"
	result := addon.Handle(core.OnInput, ctx)

	if !result.Halt {
		test.Error("expected /name to halt")
	}
	if !strings.Contains(ctx.Output, "test-session") {
		test.Errorf("expected session name in output, got: %s", ctx.Output)
	}

	// Verify name was stored
	name, ok := ctx.Get(KeySessionName)
	if !ok {
		test.Fatal("expected KeySessionName to be set")
	}
	if name != "test-session" {
		test.Errorf("expected name 'test-session', got %v", name)
	}
}

func TestCommandAddonNameShowsCurrent(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Set(KeySessionName, "existing-name")
	ctx.Input = "/name"
	addon.Handle(core.OnInput, ctx)

	if !strings.Contains(ctx.Output, "existing-name") {
		test.Errorf("expected current name in output, got: %s", ctx.Output)
	}
}

func TestCommandAddonBangPrefix(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "!help"
	result := addon.Handle(core.OnInput, ctx)

	if !result.Halt {
		test.Error("expected ! prefix to work like /")
	}
	if !strings.Contains(ctx.Output, "Commands") {
		test.Error("expected help output via ! prefix")
	}
}

func TestCommandAddonOnOutputTracksTurns(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	if addon.turnCount != 0 {
		test.Errorf("expected initial turnCount 0, got %d", addon.turnCount)
	}

	ctx := testContext()
	ctx.Output = "some response"
	addon.Handle(core.OnOutput, ctx)

	if addon.turnCount != 1 {
		test.Errorf("expected turnCount 1 after OnOutput, got %d", addon.turnCount)
	}
}

func TestCommandAddonStatusCommand(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "/status"
	addon.Handle(core.OnInput, ctx)

	if !strings.Contains(ctx.Output, "Session") {
		test.Errorf("expected status to contain 'Session', got: %s", ctx.Output)
	}
	if !strings.Contains(ctx.Output, "Turns") {
		test.Errorf("expected status to contain 'Turns', got: %s", ctx.Output)
	}
}

func TestCommandAddonThinkingToggle(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "/thinking on"
	addon.Handle(core.OnInput, ctx)

	val, ok := ctx.Get(KeyThinkingVisible)
	if !ok {
		test.Fatal("expected KeyThinkingVisible to be set")
	}
	if visible, ok := val.(bool); !ok || !visible {
		test.Error("expected thinking visible to be true")
	}
}

func TestCommandAddonRetryNoPrevious(test *testing.T) {
	dispatcher := core.NewDispatcher()
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "/retry"
	result := addon.Handle(core.OnInput, ctx)

	if !result.Halt {
		test.Error("expected /retry with no previous to halt")
	}
	if !strings.Contains(ctx.Output, "Nichts") {
		test.Errorf("expected 'Nichts zum Wiederholen', got: %s", ctx.Output)
	}
}

func TestCommandAddonDispatchesToAddonCommands(test *testing.T) {
	costguard := NewCostGuardAddon()
	costguard.Start()
	dispatcher := testDispatcher(costguard)
	addon := NewCommandAddon(dispatcher)

	ctx := testContext()
	ctx.Input = "/costs limits"
	result := addon.Handle(core.OnInput, ctx)

	if !result.Halt {
		test.Error("expected dispatched /costs to halt")
	}
	if !strings.Contains(ctx.Output, "Per turn") {
		test.Errorf("expected cost limits output, got: %s", ctx.Output)
	}
}
