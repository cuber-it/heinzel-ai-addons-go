// End-to-end integration tests — full pipeline from input through dispatcher/loop to output.

package addons

import (
	"embed"
	"strings"
	"testing"

	"github.com/cuber-it/heinzel-ai-core-go/core"
)

// setupTestAgent creates a minimal but functional agent with EchoProvider,
// CommandAddon, and PromptAddon wired together. Returns dispatcher, loop,
// and registers cleanup via t.Cleanup.
func setupTestAgent(t *testing.T) (*core.Dispatcher, *core.Loop) {
	t.Helper()
	dispatcher := core.NewDispatcher()

	dispatcher.Register(&EchoProvider{}, 100)
	dispatcher.Register(NewCommandAddon(dispatcher), 1)
	dispatcher.Register(NewPromptAddon("Test system prompt", dispatcher, nil), 5)

	if err := dispatcher.StartAll(); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	t.Cleanup(func() { dispatcher.StopAll() })

	loop := core.NewLoop(dispatcher)
	return dispatcher, loop
}

// runTurn creates a fresh context (if nil), initializes thinking, runs one turn,
// and returns the context and output.
func runTurn(t *testing.T, loop *core.Loop, ctx *core.Context, input string) (*core.Context, string) {
	t.Helper()
	if ctx == nil {
		ctx = core.NewContext("test")
		core.InitThinking(ctx, nil)
	}
	output := loop.Run(ctx, input)
	return ctx, output
}

// --- Tests ---

func TestE2E_SimpleConversation(t *testing.T) {
	_, loop := setupTestAgent(t)

	ctx, output := runTurn(t, loop, nil, "hello world")

	if output == "" {
		t.Fatal("expected non-empty output from echo provider")
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("echo provider should reflect input, got: %s", output)
	}

	// Verify messages recorded: 1 user + 1 assistant
	if len(ctx.Messages) < 2 {
		t.Errorf("expected at least 2 messages (user+assistant), got %d", len(ctx.Messages))
	}

	foundUser := false
	foundAssistant := false
	for _, msg := range ctx.Messages {
		if msg.Role == "user" && strings.Contains(msg.Content, "hello world") {
			foundUser = true
		}
		if msg.Role == "assistant" && strings.Contains(msg.Content, "hello world") {
			foundAssistant = true
		}
	}
	if !foundUser {
		t.Error("user message not found in context.Messages")
	}
	if !foundAssistant {
		t.Error("assistant message not found in context.Messages")
	}
}

func TestE2E_SlashCommand(t *testing.T) {
	_, loop := setupTestAgent(t)

	ctx, output := runTurn(t, loop, nil, "/clear")

	if !strings.Contains(output, "gelöscht") {
		t.Errorf("expected /clear confirmation, got: %s", output)
	}

	// /clear wipes messages, so after the command only the user message from the
	// current turn might remain (added before OnInput fires), but /clear sets Halt
	// so the assistant message is not appended. The command handler clears Messages.
	// Because AddMessage("user", ...) happens before dispatch, and /clear sets
	// Messages=nil, we expect Messages to be empty or only contain the clear command.
	// The key point: the echo provider was NOT called (Halt=true intercepted it).
	if strings.Contains(output, "[echo]") {
		t.Error("slash command should have been intercepted before reaching echo provider")
	}

	// Verify halt was set
	if !ctx.Halt {
		t.Error("expected Halt=true after slash command")
	}
}

func TestE2E_StrategyClassification(t *testing.T) {
	dispatcher := core.NewDispatcher()
	dispatcher.Register(&EchoProvider{}, 100)
	dispatcher.Register(NewCommandAddon(dispatcher), 1)
	dispatcher.Register(NewPromptAddon("Test system prompt", dispatcher, nil), 5)
	reasoningAddon := NewReasoningAddon(3)
	reasoningAddon.SetDispatcher(dispatcher)
	dispatcher.Register(reasoningAddon, 10)
	dispatcher.StartAll()
	t.Cleanup(func() { dispatcher.StopAll() })

	loop := core.NewLoop(dispatcher)

	ctx, _ := runTurn(t, loop, nil, "explain why water boils")

	strategyVal, ok := ctx.Get(KeyStrategy)
	if !ok {
		t.Fatal("expected strategy to be set in context")
	}
	strategy, ok := strategyVal.(Strategy)
	if !ok {
		t.Fatalf("strategy is not Strategy, got %T", strategyVal)
	}
	if strategy != StrategyChainOfThought {
		t.Errorf("expected ChainOfThought for 'explain why', got %s", strategy)
	}
}

func TestE2E_CostGuardBlocks(t *testing.T) {
	dispatcher := core.NewDispatcher()
	dispatcher.Register(&EchoProvider{}, 100)
	dispatcher.Register(NewCommandAddon(dispatcher), 1)

	costGuard := NewCostGuardAddon()
	costGuard.SetLimits(CostLimits{
		PerSession: 1, // 1 token = immediately exhausted
	})
	dispatcher.Register(costGuard, 2)
	dispatcher.StartAll()
	t.Cleanup(func() { dispatcher.StopAll() })

	loop := core.NewLoop(dispatcher)

	// First turn: CostGuard counts tokens on OnLLMResponse, so the first call
	// goes through. After it, session tokens exceed the budget of 1.
	ctx := core.NewContext("test")
	core.InitThinking(ctx, nil)
	loop.Run(ctx, "first message")

	// Second turn: budget is now exhausted, OnLLMCall should block
	ctx.Halt = false
	ctx.Error = nil
	loop.Run(ctx, "second message")

	if !ctx.Halt {
		t.Error("expected Halt=true when budget is exhausted")
	}
	if ctx.Error == nil || !strings.Contains(ctx.Error.Error(), "exhausted") {
		t.Errorf("expected budget exhaustion error, got: %v", ctx.Error)
	}
}

func TestE2E_MemoryComposer(t *testing.T) {
	dispatcher := core.NewDispatcher()
	dispatcher.Register(&EchoProvider{}, 100)
	dispatcher.Register(NewCommandAddon(dispatcher), 1)
	dispatcher.Register(NewPromptAddon("Test system prompt", dispatcher, nil), 5)

	memComposer := NewMemoryComposerAddon(16000)
	factsLayer := NewFactsLayer()
	memComposer.Composer().Register(factsLayer)
	dispatcher.Register(memComposer, 15)
	dispatcher.StartAll()
	t.Cleanup(func() { dispatcher.StopAll() })

	// Add facts before running
	factsLayer.Set("language", "Go")
	factsLayer.Set("project", "neo-heinzel")

	loop := core.NewLoop(dispatcher)
	ctx, _ := runTurn(t, loop, nil, "what do you know?")

	// After OnContextBuild, the MemoryComposer should have injected facts
	// into the prompt layers. ctx.SystemPrompt is composed after OnContextBuild.
	systemPrompt := ctx.SystemPrompt
	if !strings.Contains(systemPrompt, "language") || !strings.Contains(systemPrompt, "Go") {
		t.Errorf("expected facts in system prompt, got: %s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "project") || !strings.Contains(systemPrompt, "neo-heinzel") {
		t.Errorf("expected project fact in system prompt, got: %s", systemPrompt)
	}
}

func TestE2E_MultiTurn(t *testing.T) {
	_, loop := setupTestAgent(t)

	ctx := core.NewContext("test")
	core.InitThinking(ctx, nil)

	inputs := []string{"first message", "second message", "third message"}
	for _, input := range inputs {
		output := loop.Run(ctx, input)
		if output == "" {
			t.Fatalf("empty output for input %q", input)
		}
	}

	// Each turn: 1 user + 1 assistant = 2 messages per turn
	expectedMessages := len(inputs) * 2
	if len(ctx.Messages) != expectedMessages {
		t.Errorf("expected %d messages after %d turns, got %d",
			expectedMessages, len(inputs), len(ctx.Messages))
	}

	// Verify alternating roles
	for idx, msg := range ctx.Messages {
		expectedRole := "user"
		if idx%2 == 1 {
			expectedRole = "assistant"
		}
		if msg.Role != expectedRole {
			t.Errorf("message[%d]: expected role %q, got %q", idx, expectedRole, msg.Role)
		}
	}
}

func TestE2E_FactoryBuild(t *testing.T) {
	config := core.DefaultConfig()
	dispatcher := core.NewDispatcher()

	// The factory_register.go init() has already registered all builders via import
	var emptyFS embed.FS
	factory := core.NewFactory(config, dispatcher, emptyFS)
	result, err := factory.Build()
	if err != nil {
		t.Fatalf("Factory.Build failed: %v", err)
	}

	if len(result.Entries) == 0 {
		t.Fatal("expected at least pflicht addons from factory build")
	}

	// Check pflicht addons are present
	pflichtNames := map[string]bool{
		"commands": false, "prompt": false, "costguard": false, "echo-provider": false,
	}
	for _, entry := range result.Entries {
		if _, ok := pflichtNames[entry.Addon.Name()]; ok {
			pflichtNames[entry.Addon.Name()] = true
		}
	}
	for name, found := range pflichtNames {
		if !found {
			t.Errorf("pflicht addon %q not found in factory output", name)
		}
	}

	// Register all, StartAll, StopAll
	for _, entry := range result.Entries {
		if err := dispatcher.Register(entry.Addon, entry.Priority); err != nil {
			t.Fatalf("Register %q failed: %v", entry.Name, err)
		}
	}
	if err := dispatcher.StartAll(); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	dispatcher.StopAll() // should not panic
}

func TestE2E_SessionLifecycle(t *testing.T) {
	dispatcher := core.NewDispatcher()
	dispatcher.Register(&EchoProvider{}, 100)
	dispatcher.Register(NewCommandAddon(dispatcher), 1)
	dispatcher.Register(NewPromptAddon("Test system prompt", dispatcher, nil), 5)

	costGuard := NewCostGuardAddon()
	dispatcher.Register(costGuard, 2)

	reasoningAddon := NewReasoningAddon(3)
	reasoningAddon.SetDispatcher(dispatcher)
	dispatcher.Register(reasoningAddon, 10)

	dispatcher.StartAll()
	t.Cleanup(func() { dispatcher.StopAll() })

	loop := core.NewLoop(dispatcher)

	// Simulate a session lifecycle using Loop.Session
	var outputs []string
	turnIndex := 0
	inputs := []string{"hello", "how are you?", "goodbye"}

	loop.Session("test-session", func() (string, bool) {
		if turnIndex >= len(inputs) {
			return "", false
		}
		input := inputs[turnIndex]
		turnIndex++
		return input, true
	}, func(output string) {
		outputs = append(outputs, output)
	})

	if len(outputs) != len(inputs) {
		t.Errorf("expected %d outputs, got %d", len(inputs), len(outputs))
	}
	for idx, output := range outputs {
		if output == "" {
			t.Errorf("output[%d] is empty", idx)
		}
		if !strings.Contains(output, inputs[idx]) {
			t.Errorf("output[%d] should echo %q, got %q", idx, inputs[idx], output)
		}
	}
}
